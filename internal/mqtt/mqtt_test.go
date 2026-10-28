package mqtt

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Pyzyi/Netward/internal/config"
	"github.com/Pyzyi/Netward/internal/hub"
	"github.com/Pyzyi/Netward/internal/persons"
	"github.com/Pyzyi/Netward/internal/store"
)

// fakeBroker implements just enough MQTT 3.1.1 to exercise the client.
type fakeBroker struct {
	ln       net.Listener
	mu       sync.Mutex
	retained map[string]string
	connect  []byte
	conns    []net.Conn
}

func newBroker(t *testing.T) *fakeBroker {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	b := &fakeBroker{ln: ln, retained: map[string]string{}}
	t.Cleanup(func() {
		ln.Close()
		b.mu.Lock()
		for _, c := range b.conns {
			c.Close()
		}
		b.mu.Unlock()
	})
	go b.serve()
	return b
}

func (b *fakeBroker) serve() {
	for {
		c, err := b.ln.Accept()
		if err != nil {
			return
		}
		b.mu.Lock()
		b.conns = append(b.conns, c)
		b.mu.Unlock()
		go b.handle(c)
	}
}

func (b *fakeBroker) handle(c net.Conn) {
	r := bufio.NewReader(c)
	for {
		h, body, err := readPacket(r)
		if err != nil {
			return
		}
		switch h >> 4 {
		case pConnect:
			b.mu.Lock()
			b.connect = body
			b.mu.Unlock()
			_, _ = c.Write([]byte{pConnack << 4, 2, 0, 0})
		case pPublish:
			tl := int(binary.BigEndian.Uint16(body[0:2]))
			topic := string(body[2 : 2+tl])
			id := body[2+tl : 4+tl]
			payload := string(body[4+tl:])
			if h&0x01 != 0 {
				b.mu.Lock()
				b.retained[topic] = payload
				b.mu.Unlock()
			}
			_, _ = c.Write(packet(pPuback<<4, append([]byte(nil), id...)))
		case pSubscribe:
			_, _ = c.Write(packet(pSuback<<4, []byte{body[0], body[1], 1}))
			// Deliver retained messages for "<prefix>+/config" filters.
			fl := int(binary.BigEndian.Uint16(body[2:4]))
			if filter := string(body[4 : 4+fl]); strings.HasSuffix(filter, "+/config") {
				prefix := strings.TrimSuffix(filter, "+/config")
				b.mu.Lock()
				for topic, v := range b.retained {
					if strings.HasPrefix(topic, prefix) && strings.HasSuffix(topic, "/config") && v != "" {
						_, _ = c.Write(packet(pPublish<<4|1, append(appendString(nil, topic), v...)))
					}
				}
				b.mu.Unlock()
			}
			// Home Assistant announces itself right after a subscription.
			msg := appendString(nil, "homeassistant/status")
			msg = append(msg, "online"...)
			_, _ = c.Write(packet(pPublish<<4, msg))
		case pPingreq:
			_, _ = c.Write([]byte{pPingresp << 4, 0})
		case pDisconnect:
			c.Close()
			return
		}
	}
}

func (b *fakeBroker) get(topic string) (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	v, ok := b.retained[topic]
	return v, ok
}

func TestClientRoundTrip(t *testing.T) {
	b := newBroker(t)
	got := make(chan string, 1)
	cl, err := Connect(context.Background(), Options{Addr: b.ln.Addr().String(), ClientID: "test", Username: "u", Password: "p",
		WillTopic: "nw/status", WillMsg: "offline", KeepAlive: 2 * time.Second,
		OnMessage: func(topic string, payload []byte) { got <- topic + "=" + string(payload) }})
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	b.mu.Lock()
	connect := b.connect
	b.mu.Unlock()
	if !strings.Contains(string(connect), "nw/status") || !strings.Contains(string(connect), "offline") {
		t.Fatal("last will missing in CONNECT")
	}
	if err := cl.Publish(context.Background(), "nw/x", []byte("1"), true); err != nil {
		t.Fatal(err)
	}
	if v, _ := b.get("nw/x"); v != "1" {
		t.Fatal("retained publish not stored")
	}
	if err := cl.Subscribe(context.Background(), "homeassistant/status"); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if m != "homeassistant/status=online" {
			t.Fatalf("unexpected message %q", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message delivered")
	}
	// Keep-alive must keep the connection open across several intervals.
	time.Sleep(2500 * time.Millisecond)
	select {
	case <-cl.Done():
		t.Fatalf("connection dropped: %v", cl.Err())
	default:
	}
}

func TestConnRefused(t *testing.T) {
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		r := bufio.NewReader(c)
		_, _, _ = readPacket(r)
		_, _ = c.Write([]byte{pConnack << 4, 2, 0, 5})
	}()
	_, err := Connect(context.Background(), Options{Addr: ln.Addr().String(), ClientID: "x"})
	if _, ok := err.(ErrConnRefused); !ok {
		t.Fatalf("want ErrConnRefused, got %v", err)
	}
}

type fakeSource struct{ devs []store.Device }

func (f fakeSource) Devices() []store.Device { return f.devs }

func TestBridgePublishesWatchedDevices(t *testing.T) {
	b := newBroker(t)
	host, port, _ := net.SplitHostPort(b.ln.Addr().String())
	src := fakeSource{devs: []store.Device{
		{ID: 1, Ident: "3c:22:fb:00:00:01", MAC: "3c:22:fb:00:00:01", Hostname: "Annas-iPhone", Watch: true, Online: true, Vendor: "Apple"},
		{ID: 2, Ident: "3c:22:fb:00:00:02", MAC: "3c:22:fb:00:00:02", Watch: false},
	}}
	// Loopback brokers are only reachable with a permissive dialer in tests.
	br := NewBridge(src, hub.New(), nil, nil, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	cfg := config.MQTT{Enabled: true, Host: host, DiscoveryPrefix: "homeassistant", BaseTopic: "netzwart"}
	cfg.Port = mustAtoi(port)
	br.Configure(cfg, "de")
	br.dial = (&net.Dialer{}).DialContext
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go br.Run(ctx)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && br.Status().State != "connected" {
		time.Sleep(20 * time.Millisecond)
	}
	if br.Status().State != "connected" {
		t.Fatalf("bridge not connected: %+v", br.Status())
	}
	id := ObjectID("3c:22:fb:00:00:01")
	if id != "netzwart_3c22fb000001" {
		t.Fatalf("unexpected object id %q", id)
	}
	cfgMsg, ok := b.get("homeassistant/device_tracker/" + id + "/config")
	if !ok || !strings.Contains(cfgMsg, `"payload_home":"home"`) || !strings.Contains(cfgMsg, "Annas-iPhone") {
		t.Fatalf("discovery config wrong: %q", cfgMsg)
	}
	if st, _ := b.get("netzwart/device/" + id + "/state"); st != "home" {
		t.Fatalf("state = %q", st)
	}
	if _, ok := b.get("homeassistant/device_tracker/" + ObjectID("3c:22:fb:00:00:02") + "/config"); ok {
		t.Fatal("unwatched device published")
	}
	if v, _ := b.get("netzwart/status"); v != "online" {
		t.Fatal("availability not online")
	}
	if v, _ := b.get("netzwart/stats/total"); v != "2" {
		t.Fatalf("stats total = %q", v)
	}
}

// Review 2: switching brokers resends discovery configs, and entities of
// devices that are no longer watched are cleared from the broker.
func TestBridgeResyncsAfterBrokerChange(t *testing.T) {
	b1, b2 := newBroker(t), newBroker(t)
	stale := "homeassistant/device_tracker/netzwart_aaaaaaaaaaaa/config"
	b2.mu.Lock()
	b2.retained[stale] = `{"name":null}`
	b2.mu.Unlock()
	src := fakeSource{devs: []store.Device{{ID: 1, Ident: "3c:22:fb:00:00:01", MAC: "3c:22:fb:00:00:01", Watch: true, Online: true}}}
	br := NewBridge(src, hub.New(), nil, nil, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	br.dial = (&net.Dialer{}).DialContext
	cfgFor := func(b *fakeBroker) config.MQTT {
		host, port, _ := net.SplitHostPort(b.ln.Addr().String())
		return config.MQTT{Enabled: true, Host: host, Port: mustAtoi(port), DiscoveryPrefix: "homeassistant", BaseTopic: "netzwart"}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	br.Configure(cfgFor(b1), "de")
	go br.Run(ctx)
	topic := "homeassistant/device_tracker/" + ObjectID("3c:22:fb:00:00:01") + "/config"
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			if ok() {
				return
			}
		}
		t.Fatal(what)
	}
	waitFor("config on first broker", func() bool { _, ok := b1.get(topic); return ok })
	br.Configure(cfgFor(b2), "de")
	waitFor("config not resent to the new broker", func() bool { _, ok := b2.get(topic); return ok })
	waitFor("stale entity not cleared", func() bool { v, _ := b2.get(stale); return v == "" })
}

func mustAtoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

type fakePeople struct {
	mu sync.Mutex
	st []persons.State
}

func (f *fakePeople) States() []persons.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]persons.State(nil), f.st...)
}

func TestBridgePublishesPersons(t *testing.T) {
	b := newBroker(t)
	host, port, _ := net.SplitHostPort(b.ln.Addr().String())
	h := hub.New()
	br := NewBridge(fakeSource{}, h, nil, nil, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	people := &fakePeople{st: []persons.State{{ID: 1, Name: "Anna", Home: true, HA: true}, {ID: 2, Name: "Max", HA: false}}}
	br.SetPersons(people)
	cfg := config.MQTT{Enabled: true, Host: host, DiscoveryPrefix: "homeassistant", BaseTopic: "netzwart"}
	cfg.Port = mustAtoi(port)
	br.Configure(cfg, "de")
	br.dial = (&net.Dialer{}).DialContext
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go br.Run(ctx)
	wait := func(cond func() bool) bool {
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			if cond() {
				return true
			}
		}
		return false
	}
	if !wait(func() bool { v, _ := b.get("netzwart/person/1/state"); return v == "home" }) {
		t.Fatal("person state not published")
	}
	cfgMsg, _ := b.get("homeassistant/device_tracker/netzwart_person_1/config")
	if !strings.Contains(cfgMsg, `"unique_id":"netzwart_person_1"`) || !strings.Contains(cfgMsg, `"name":"Anna"`) {
		t.Fatalf("config %q", cfgMsg)
	}
	if _, ok := b.get("homeassistant/device_tracker/netzwart_person_2/config"); ok {
		t.Fatal("person hidden from Home Assistant was published")
	}
	// Anna leaves, then is deleted: state follows, then the entity goes.
	people.mu.Lock()
	people.st[0].Home = false
	people.mu.Unlock()
	h.Publish("persons", nil)
	if !wait(func() bool { v, _ := b.get("netzwart/person/1/state"); return v == "not_home" }) {
		t.Fatal("state not updated")
	}
	people.mu.Lock()
	people.st = people.st[1:]
	people.mu.Unlock()
	h.Publish("persons", nil)
	if !wait(func() bool {
		v, ok := b.get("homeassistant/device_tracker/netzwart_person_1/config")
		return !ok || v == ""
	}) {
		t.Fatal("deleted person still has an entity")
	}
}
