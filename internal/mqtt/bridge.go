package mqtt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Pyzyi/netzwart/internal/config"
	"github.com/Pyzyi/netzwart/internal/egress"
	"github.com/Pyzyi/netzwart/internal/hub"
	"github.com/Pyzyi/netzwart/internal/internet"
	"github.com/Pyzyi/netzwart/internal/notify"
	"github.com/Pyzyi/netzwart/internal/persons"
	"github.com/Pyzyi/netzwart/internal/store"
)

// Source provides the data the bridge publishes.
type Source interface {
	Devices() []store.Device
}

// PersonSource provides the persons (docs/09, F4).
type PersonSource interface {
	States() []persons.State
}

// BridgeStatus is shown in the settings page.
type BridgeStatus struct {
	State string `json:"state"` // disabled | connecting | connected | error
	Error string `json:"error,omitempty"`
	Since int64  `json:"since"`
}

// Bridge publishes watched devices as Home Assistant device_trackers.
type Bridge struct {
	src     Source
	hub     *hub.Hub
	policy  *egress.Policy
	log     *slog.Logger
	version string
	inet    func() internet.Status
	dial    func(ctx context.Context, network, addr string) (net.Conn, error)

	mu        sync.Mutex
	cfg       config.MQTT
	lang      string
	reconnect chan struct{}
	status    BridgeStatus
	published map[string]bool // object IDs with a discovery config
	people    PersonSource
	pubNames  map[string]string // person object ID -> name in its published config
	pubStates map[string]string // person object ID -> last published state
}

// SetPersons adds a device_tracker per person.
func (b *Bridge) SetPersons(p PersonSource) {
	b.mu.Lock()
	b.people = p
	b.mu.Unlock()
}

func personObjectID(id int64) string { return "netzwart_person_" + strconv.FormatInt(id, 10) }

func (b *Bridge) personStates() []persons.State {
	b.mu.Lock()
	p := b.people
	b.mu.Unlock()
	if p == nil {
		return nil
	}
	return p.States()
}

// wanted are the entities that should exist: watched devices and the
// persons shown in Home Assistant.
func (b *Bridge) wanted() map[string]bool {
	ids := map[string]bool{}
	for _, d := range b.src.Devices() {
		if d.Watch {
			ids[deviceObjectID(d)] = true
		}
	}
	for _, p := range b.personStates() {
		if p.HA {
			ids[personObjectID(p.ID)] = true
		}
	}
	return ids
}

// NewBridge creates the bridge; Run must be called to start it.
func NewBridge(src Source, h *hub.Hub, policy *egress.Policy, inet func() internet.Status, version string, log *slog.Logger) *Bridge {
	b := &Bridge{src: src, hub: h, policy: policy, inet: inet, version: version, log: log,
		reconnect: make(chan struct{}, 1), status: BridgeStatus{State: "disabled"}, published: map[string]bool{}, pubNames: map[string]string{}, pubStates: map[string]string{}}
	if policy != nil {
		b.dial = policy.DialContext
	}
	return b
}

// Configure applies new MQTT settings and reconnects.
func (b *Bridge) Configure(cfg config.MQTT, lang string) {
	b.mu.Lock()
	changed := cfg != b.cfg
	b.cfg, b.lang = cfg, lang
	b.mu.Unlock()
	if changed {
		select {
		case b.reconnect <- struct{}{}:
		default:
		}
	}
}

// Status returns the connection state.
func (b *Bridge) Status() BridgeStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status
}

func (b *Bridge) setStatus(state, errMsg string) {
	b.mu.Lock()
	if b.status.State != state || b.status.Error != errMsg {
		b.status = BridgeStatus{State: state, Error: errMsg, Since: time.Now().UnixMilli()}
	}
	st := b.status
	b.mu.Unlock()
	b.hub.Publish("mqtt", st)
}

// ObjectID derives a stable, topic-safe ID from a device identity.
func ObjectID(ident string) string {
	var sb strings.Builder
	sb.WriteString("netzwart_")
	for _, r := range strings.ToLower(ident) {
		switch {
		case r >= 'a' && r <= 'f', r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == '/' || r == '.':
			sb.WriteByte('_')
		}
	}
	return sb.String()
}

func deviceObjectID(d store.Device) string {
	if d.Ident != "" {
		return ObjectID(d.Ident)
	}
	return ObjectID(d.MAC)
}

// Run maintains the connection until ctx ends.
func (b *Bridge) Run(ctx context.Context) {
	backoff := 5 * time.Second
	for {
		// A pending "settings changed" is answered by reading them now; left
		// in the channel it would end the session right after connecting.
		select {
		case <-b.reconnect:
		default:
		}
		b.mu.Lock()
		cfg := b.cfg
		b.mu.Unlock()
		if !cfg.Enabled {
			b.setStatus("disabled", "")
			select {
			case <-ctx.Done():
				return
			case <-b.reconnect:
				continue
			}
		}
		b.setStatus("connecting", "")
		err := b.session(ctx, cfg)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			b.setStatus("error", err.Error())
			b.log.Warn("mqtt connection failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-b.reconnect:
			backoff = 5 * time.Second
		case <-time.After(backoff):
			backoff = min(backoff*2, 5*time.Minute)
		}
	}
}

type deviceInfo struct {
	Identifiers  []string   `json:"identifiers"`
	Name         string     `json:"name"`
	Manufacturer string     `json:"manufacturer,omitempty"`
	Model        string     `json:"model,omitempty"`
	Connections  [][]string `json:"connections,omitempty"`
	ViaDevice    string     `json:"via_device,omitempty"`
	SWVersion    string     `json:"sw_version,omitempty"`
}

func (b *Bridge) session(ctx context.Context, cfg config.MQTT) error {
	rnd := make([]byte, 4)
	_, _ = rand.Read(rnd)
	base := cfg.BaseTopic
	haStatus := cfg.DiscoveryPrefix + "/status"
	resync := make(chan struct{}, 1)
	// Discovery configs are tracked per broker session: a new broker (or one
	// without persistence) has none of them yet.
	b.mu.Lock()
	b.published, b.pubNames, b.pubStates = map[string]bool{}, map[string]string{}, map[string]string{}
	b.mu.Unlock()
	// Retained configs the broker still holds from earlier runs; entities of
	// devices that are no longer watched are removed after the initial sync.
	trackerPrefix := cfg.DiscoveryPrefix + "/device_tracker/"
	var retainedMu sync.Mutex
	retained := map[string]bool{}
	cl, err := Connect(ctx, Options{
		Addr: net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)), TLS: cfg.TLS,
		ClientID: "netzwart-" + hex.EncodeToString(rnd), Username: cfg.Username, Password: string(cfg.Password),
		KeepAlive: 60 * time.Second, WillTopic: base + "/status", WillMsg: "offline",
		Dial: b.dial,
		OnMessage: func(topic string, payload []byte) {
			if topic == haStatus && string(payload) == "online" {
				select {
				case resync <- struct{}{}:
				default:
				}
			}
			if id, ok := strings.CutSuffix(strings.TrimPrefix(topic, trackerPrefix), "/config"); ok &&
				strings.HasPrefix(topic, trackerPrefix) && strings.HasPrefix(id, "netzwart_") {
				retainedMu.Lock()
				if len(payload) > 0 && len(retained) < 10000 {
					retained[id] = true
				} else {
					delete(retained, id)
				}
				retainedMu.Unlock()
			}
		},
	})
	if err != nil {
		return err
	}
	defer cl.Close()
	sub := b.hub.Subscribe(256)
	defer b.hub.Unsubscribe(sub)
	if err := cl.Publish(ctx, base+"/status", []byte("online"), true); err != nil {
		return err
	}
	if err := cl.Subscribe(ctx, haStatus); err != nil {
		return err
	}
	if err := cl.Subscribe(ctx, trackerPrefix+"+/config"); err != nil {
		return err
	}
	if err := b.publishAll(ctx, cl, cfg); err != nil {
		return err
	}
	cleanup := time.NewTimer(3 * time.Second) // time for the broker to deliver retained configs
	defer cleanup.Stop()
	b.setStatus("connected", "")
	b.log.Info("mqtt connected", "broker", cfg.Host)
	stats := time.NewTicker(time.Minute)
	defer stats.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = cl.Publish(context.Background(), base+"/status", []byte("offline"), true)
			return nil
		case <-b.reconnect:
			return nil
		case <-cl.Done():
			return cl.Err()
		case <-cleanup.C:
			watched := b.wanted()
			retainedMu.Lock()
			var stale []string
			for id := range retained {
				if !watched[id] {
					stale = append(stale, id)
				}
			}
			retainedMu.Unlock()
			for _, id := range stale {
				if err := b.remove(ctx, cl, cfg, id); err != nil {
					return err
				}
			}
		case <-resync:
			// Home Assistant restarted: send every config again.
			b.mu.Lock()
			b.published, b.pubNames, b.pubStates = map[string]bool{}, map[string]string{}, map[string]string{}
			b.mu.Unlock()
			if err := b.publishAll(ctx, cl, cfg); err != nil {
				return err
			}
		case <-stats.C:
			if sub.Dropped() {
				if err := b.publishAll(ctx, cl, cfg); err != nil {
					return err
				}
				continue
			}
			if err := b.publishStats(ctx, cl, cfg); err != nil {
				return err
			}
		case m := <-sub.C:
			var err error
			switch m.Type {
			case "device":
				if d, ok := m.Data.(store.Device); ok {
					err = b.publishDevice(ctx, cl, cfg, d)
				}
				if err == nil {
					err = b.publishPersons(ctx, cl, cfg)
				}
			case "persons":
				err = b.publishPersons(ctx, cl, cfg)
			case "device_removed":
				if v, ok := m.Data.(map[string]int64); ok {
					err = b.removeByID(ctx, cl, cfg, v["id"])
				}
			case "internet":
				err = b.publishStats(ctx, cl, cfg)
			}
			if err != nil {
				return err
			}
		}
	}
}

func (b *Bridge) language() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lang
}

func (b *Bridge) publishJSON(ctx context.Context, cl *Client, topic string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return cl.Publish(ctx, topic, raw, true)
}

func (b *Bridge) publishAll(ctx context.Context, cl *Client, cfg config.MQTT) error {
	base, prefix := cfg.BaseTopic, cfg.DiscoveryPrefix
	hubDev := deviceInfo{Identifiers: []string{"netzwart"}, Name: "Netzwart", Manufacturer: "Netzwart", SWVersion: b.version}
	lang := b.language()
	t := func(de, en string) string {
		if lang == "en" {
			return en
		}
		return de
	}
	sensors := []struct {
		comp, id, name, topic, unit, class string
	}{
		{"sensor", "netzwart_devices_online", t("Geräte online", "Devices online"), base + "/stats/online", "", ""},
		{"sensor", "netzwart_devices_total", t("Geräte gesamt", "Devices total"), base + "/stats/total", "", ""},
		{"sensor", "netzwart_devices_new", t("Neue Geräte", "New devices"), base + "/stats/new", "", ""},
		{"sensor", "netzwart_internet_latency", t("Internet-Latenz", "Internet latency"), base + "/internet/latency", "ms", "duration"},
		{"binary_sensor", "netzwart_internet", "Internet", base + "/internet/state", "", "connectivity"},
	}
	for _, s := range sensors {
		cfgMsg := map[string]any{"name": s.name, "unique_id": s.id, "object_id": s.id, "state_topic": s.topic,
			"availability_topic": base + "/status", "device": hubDev}
		if s.unit != "" {
			cfgMsg["unit_of_measurement"] = s.unit
		}
		if s.class != "" {
			cfgMsg["device_class"] = s.class
		}
		if s.comp == "binary_sensor" {
			cfgMsg["payload_on"], cfgMsg["payload_off"] = "ON", "OFF"
		} else {
			cfgMsg["state_class"] = "measurement"
		}
		if err := b.publishJSON(ctx, cl, prefix+"/"+s.comp+"/"+s.id+"/config", cfgMsg); err != nil {
			return err
		}
	}
	for _, d := range b.src.Devices() {
		if err := b.publishDevice(ctx, cl, cfg, d); err != nil {
			return err
		}
	}
	if err := b.publishPersons(ctx, cl, cfg); err != nil {
		return err
	}
	return b.publishStats(ctx, cl, cfg)
}

// publishPersons sends a device_tracker per person (home / not_home). IDs
// come from the person's number, never from the name; a person that is
// deleted or hidden from Home Assistant loses its entity.
func (b *Bridge) publishPersons(ctx context.Context, cl *Client, cfg config.MQTT) error {
	states := b.personStates()
	want := map[string]bool{}
	for _, p := range states {
		id := personObjectID(p.ID)
		if !p.HA {
			continue
		}
		want[id] = true
		stateTopic := cfg.BaseTopic + "/person/" + strconv.FormatInt(p.ID, 10) + "/state"
		b.mu.Lock()
		same := b.published[id] && b.pubNames[id] == p.Name
		b.mu.Unlock()
		if !same {
			msg := map[string]any{
				"name": nil, "unique_id": id, "object_id": id, "state_topic": stateTopic,
				"payload_home": "home", "payload_not_home": "not_home", "source_type": "router",
				"availability_topic": cfg.BaseTopic + "/status",
				"device":             deviceInfo{Identifiers: []string{id}, Name: p.Name, Manufacturer: "Netzwart", ViaDevice: "netzwart"},
			}
			if err := b.publishJSON(ctx, cl, cfg.DiscoveryPrefix+"/device_tracker/"+id+"/config", msg); err != nil {
				return err
			}
			b.mu.Lock()
			b.published[id], b.pubNames[id] = true, p.Name
			b.mu.Unlock()
		}
		state := "not_home"
		if p.Home {
			state = "home"
		}
		b.mu.Lock()
		unchanged := b.pubStates[id] == state
		b.mu.Unlock()
		if unchanged {
			continue
		}
		if err := cl.Publish(ctx, stateTopic, []byte(state), true); err != nil {
			return err
		}
		b.mu.Lock()
		b.pubStates[id] = state
		b.mu.Unlock()
	}
	b.mu.Lock()
	var stale []string
	for id := range b.published {
		if strings.HasPrefix(id, "netzwart_person_") && !want[id] {
			stale = append(stale, id)
		}
	}
	b.mu.Unlock()
	for _, id := range stale {
		if err := b.remove(ctx, cl, cfg, id); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bridge) publishStats(ctx context.Context, cl *Client, cfg config.MQTT) error {
	online, total, fresh := 0, 0, 0
	for _, d := range b.src.Devices() {
		total++
		if d.Online {
			online++
		}
		if d.New {
			fresh++
		}
	}
	base := cfg.BaseTopic
	for topic, v := range map[string]int{"/stats/online": online, "/stats/total": total, "/stats/new": fresh} {
		if err := cl.Publish(ctx, base+topic, []byte(strconv.Itoa(v)), true); err != nil {
			return err
		}
	}
	if b.inet != nil {
		st := b.inet()
		state := "OFF"
		if st.State == internet.StateOnline || st.State == internet.StateDegraded {
			state = "ON"
		}
		if st.State != internet.StateDisabled && st.State != internet.StateUnknown {
			if err := cl.Publish(ctx, base+"/internet/state", []byte(state), true); err != nil {
				return err
			}
			if st.LatencyMS >= 0 {
				if err := cl.Publish(ctx, base+"/internet/latency", []byte(strconv.Itoa(st.LatencyMS)), true); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (b *Bridge) publishDevice(ctx context.Context, cl *Client, cfg config.MQTT, d store.Device) error {
	id := deviceObjectID(d)
	cfgTopic := cfg.DiscoveryPrefix + "/device_tracker/" + id + "/config"
	b.mu.Lock()
	known := b.published[id]
	b.mu.Unlock()
	if !d.Watch {
		if known {
			return b.remove(ctx, cl, cfg, id)
		}
		return nil
	}
	stateTopic := cfg.BaseTopic + "/device/" + id + "/state"
	attrTopic := cfg.BaseTopic + "/device/" + id + "/attributes"
	name := notify.DeviceName(&d, b.language())
	msg := map[string]any{
		"name": nil, "unique_id": id, "object_id": id, "state_topic": stateTopic, "json_attributes_topic": attrTopic,
		"payload_home": "home", "payload_not_home": "not_home", "source_type": "router",
		"availability_topic": cfg.BaseTopic + "/status",
		"device": deviceInfo{Identifiers: []string{id}, Name: name, Manufacturer: d.Vendor, Model: d.Model,
			Connections: [][]string{{"mac", d.MAC}}, ViaDevice: "netzwart"},
	}
	if !known {
		if err := b.publishJSON(ctx, cl, cfgTopic, msg); err != nil {
			return err
		}
		b.mu.Lock()
		b.published[id] = true
		b.mu.Unlock()
	}
	state := "not_home"
	if d.Online {
		state = "home"
	}
	if err := cl.Publish(ctx, stateTopic, []byte(state), true); err != nil {
		return err
	}
	return b.publishJSON(ctx, cl, attrTopic, map[string]any{"ip": d.IP, "mac": d.MAC, "host_name": d.Hostname,
		"vendor": d.Vendor, "type": d.Type, "last_seen": time.UnixMilli(d.LastSeen).UTC().Format(time.RFC3339)})
}

func (b *Bridge) remove(ctx context.Context, cl *Client, cfg config.MQTT, id string) error {
	// An empty retained config removes the entity in Home Assistant.
	if err := cl.Publish(ctx, cfg.DiscoveryPrefix+"/device_tracker/"+id+"/config", nil, true); err != nil {
		return err
	}
	if pid, ok := strings.CutPrefix(id, "netzwart_person_"); ok {
		_ = cl.Publish(ctx, cfg.BaseTopic+"/person/"+pid+"/state", nil, true)
	} else {
		_ = cl.Publish(ctx, cfg.BaseTopic+"/device/"+id+"/state", nil, true)
		_ = cl.Publish(ctx, cfg.BaseTopic+"/device/"+id+"/attributes", nil, true)
	}
	b.mu.Lock()
	delete(b.published, id)
	delete(b.pubNames, id)
	delete(b.pubStates, id)
	b.mu.Unlock()
	return nil
}

func (b *Bridge) removeByID(ctx context.Context, cl *Client, cfg config.MQTT, deviceID int64) error {
	// The device is gone from the monitor; remove every entity that no longer matches a watched device.
	current := b.wanted()
	b.mu.Lock()
	var stale []string
	for id := range b.published {
		if !current[id] {
			stale = append(stale, id)
		}
	}
	b.mu.Unlock()
	for _, id := range stale {
		if err := b.remove(ctx, cl, cfg, id); err != nil {
			return err
		}
	}
	return nil
}
