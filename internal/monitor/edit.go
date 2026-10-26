package monitor

import (
	"context"
	"errors"
	"net/netip"
	"strings"

	"github.com/Pyzyi/netzwart/internal/classify"
	"github.com/Pyzyi/netzwart/internal/proto"
	"github.com/Pyzyi/netzwart/internal/software"
	"github.com/Pyzyi/netzwart/internal/store"
	"github.com/Pyzyi/netzwart/internal/textutil"
)

// ErrInvalid is returned for invalid user input.
var ErrInvalid = errors.New("invalid input")

// PatchDevice applies user edits after validating them.
func (m *Monitor) PatchDevice(ctx context.Context, id int64, p store.DevicePatch) (store.Device, error) {
	clean := func(v *string, max int) error {
		if v == nil {
			return nil
		}
		c := textutil.Clean(*v, max)
		if len([]rune(strings.TrimSpace(*v))) > max {
			return ErrInvalid
		}
		*v = c
		return nil
	}
	if err := errors.Join(clean(p.Name, 64), clean(p.Location, 64)); err != nil {
		return store.Device{}, err
	}
	if p.Notes != nil {
		if len([]rune(*p.Notes)) > 2000 {
			return store.Device{}, ErrInvalid
		}
		n := strings.Map(func(r rune) rune {
			if r == '\n' || r == '\t' || r >= 0x20 && r != 0x7f {
				return r
			}
			return -1
		}, *p.Notes)
		p.Notes = &n
	}
	if p.TypeOverride != nil && *p.TypeOverride != "" && !classify.Valid(*p.TypeOverride) {
		return store.Device{}, ErrInvalid
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.byID[id]
	if e == nil {
		return store.Device{}, store.ErrNotFound
	}
	if err := m.st.PatchDevice(ctx, id, p); err != nil {
		return store.Device{}, err
	}
	if p.Name != nil {
		e.d.Name = *p.Name
	}
	if p.TypeOverride != nil {
		e.d.TypeOverride = *p.TypeOverride
	}
	if p.Location != nil {
		e.d.Location = *p.Location
	}
	if p.Notes != nil {
		e.d.Notes = *p.Notes
	}
	if p.Known != nil {
		e.d.Known = *p.Known
	}
	if p.Favorite != nil {
		e.d.Favorite = *p.Favorite
	}
	if p.Watch != nil {
		e.d.Watch = *p.Watch
	}
	m.reclassify(e)
	m.publish(e)
	return m.view(e), nil
}

// AcknowledgeAll marks every device as known.
func (m *Monitor) AcknowledgeAll(ctx context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids, err := m.st.AcknowledgeAll(ctx)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if e := m.byID[id]; e != nil {
			e.d.Known = true
			m.publish(e)
		}
	}
	return len(ids), nil
}

// DeleteDevice forgets a device. If it is still on the network it will
// reappear (as new) with the next sighting.
func (m *Monitor) DeleteDevice(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.byID[id]
	if e == nil {
		return store.ErrNotFound
	}
	snap := m.view(e)
	if err := m.st.DeleteDevice(ctx, id); err != nil {
		return err
	}
	m.unindex(e)
	delete(m.dirty, id)
	m.addEvent(ctx, KindDeviceRemoved, "info", nil, map[string]any{"name": displayName(snap), "ip": snap.IP, "mac": snap.MAC})
	m.hub.Publish("device_removed", map[string]int64{"id": id})
	return nil
}

// MergeDevices folds device from into device into (e.g. after a phone
// rotated its private MAC address).
func (m *Monitor) MergeDevices(ctx context.Context, from, into int64) (store.Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	src, dst := m.byID[from], m.byID[into]
	if src == nil || dst == nil {
		return store.Device{}, store.ErrNotFound
	}
	if from == into {
		return store.Device{}, ErrInvalid
	}
	if err := m.st.MergeDevices(ctx, from, into); err != nil {
		return store.Device{}, err
	}
	fresh, err := m.st.GetDevice(ctx, into)
	if err != nil {
		return store.Device{}, err
	}
	// Keep live state (online, addresses) of the target and adopt user data.
	fresh.Online = dst.d.Online
	fresh.LastSeen = max(dst.d.LastSeen, fresh.LastSeen)
	dst.d.Name, dst.d.Notes, dst.d.Location = fresh.Name, fresh.Notes, fresh.Location
	dst.d.TypeOverride, dst.d.Known, dst.d.Favorite, dst.d.Watch = fresh.TypeOverride, fresh.Known, fresh.Favorite, fresh.Watch
	dst.d.FirstSeen = fresh.FirstSeen
	for k, v := range src.names {
		if dst.names[k] == "" {
			dst.names[k] = v
		}
	}
	m.unindex(src)
	m.index(dst)
	delete(m.dirty, from)
	m.reclassify(dst)
	m.persist(ctx, dst)
	v := m.view(dst)
	m.addEvent(ctx, KindMerged, "info", &v, map[string]any{"from": displayName(src.d), "fromMac": src.d.MAC})
	m.hub.Publish("device_removed", map[string]int64{"id": from})
	m.publish(dst)
	return v, nil
}

// Enrichment is what active name resolution found out about a device.
type Enrichment struct {
	DNS          string
	NetBIOS      string
	MDNS         *proto.MDNSInfo
	UPnP         *proto.UPnPDevice
	FritzBoxName string
	SSDPServer   string // SERVER header of an SSDP response
	At           int64
}

// ApplyEnrichment merges resolved names and services into a device.
func (m *Monitor) ApplyEnrichment(ctx context.Context, id int64, en Enrichment) {
	m.applyEnrichment(ctx, id, en)
	m.softwareChanged()
}

func (m *Monitor) applyEnrichment(ctx context.Context, id int64, en Enrichment) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.byID[id]
	if e == nil {
		return
	}
	set := func(src, v string) {
		if v != "" {
			e.names[src] = v
		}
	}
	set("dns", textutil.Hostname(en.DNS))
	set("netbios", textutil.Clean(en.NetBIOS, 64))
	set("fritzbox", textutil.Clean(en.FritzBoxName, 64))
	if en.MDNS != nil {
		m.applyMDNS(e, en.MDNS)
	}
	if en.SSDPServer != "" {
		m.observeSoftware(e, software.FromSSDPServer(en.SSDPServer))
	}
	if en.UPnP != nil {
		set("ssdp", textutil.Clean(en.UPnP.FriendlyName, 64))
		e.upnpMaker = textutil.Clean(en.UPnP.Manufacturer, 64)
		e.upnpTypes = e.upnpTypes[:0]
		for _, t := range en.UPnP.AllTypes() {
			if t = textutil.Clean(t, 128); t != "" && len(e.upnpTypes) < 16 {
				e.upnpTypes = append(e.upnpTypes, t)
			}
		}
		if mdl := strings.TrimSpace(textutil.Clean(en.UPnP.ModelName, 48) + " " + textutil.Clean(en.UPnP.ModelNumber, 24)); mdl != "" && e.model == "" {
			e.model = mdl
		}
		svcs := make([]store.Service, 0, len(e.upnpTypes))
		for _, t := range e.upnpTypes {
			svcs = append(svcs, store.Service{Source: "ssdp", Service: t, Detail: e.upnpMaker})
		}
		_ = m.st.ReplaceServices(ctx, id, "ssdp", svcs)
	}
	if en.At > 0 {
		e.d.EnrichedAt = en.At
	}
	m.reclassify(e)
	m.persist(ctx, e)
	m.publish(e)
}

// SetPorts records the open ports found by a port scan.
func (m *Monitor) SetPorts(ctx context.Context, id int64, ports []store.Port, at int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.byID[id]
	if e == nil {
		return store.ErrNotFound
	}
	if err := m.st.ReplacePorts(ctx, id, ports, at); err != nil {
		return err
	}
	e.ports = e.ports[:0]
	for _, p := range ports {
		e.ports = append(e.ports, p.Port)
	}
	e.d.PortsScannedAt = at
	m.reclassify(e)
	m.persist(ctx, e)
	m.publish(e)
	return nil
}

// NeedsEnrichment returns online devices whose names are older than maxAge.
func (m *Monitor) NeedsEnrichment(olderThan int64) []int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	var ids []int64
	for id, e := range m.byID {
		if e.d.Online && e.d.EnrichedAt < olderThan {
			ids = append(ids, id)
		}
	}
	return ids
}

// Addresses returns the IPs a device answered on recently.
func (m *Monitor) Addresses(id int64) []netip.Addr {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.byID[id]
	if e == nil {
		return nil
	}
	out := make([]netip.Addr, 0, len(e.addrs))
	for ip := range e.addrs {
		out = append(out, ip)
	}
	return out
}

// Details returns discovery details for the device detail view.
type Details struct {
	Names     map[string]string `json:"names"`
	Services  []string          `json:"services"`
	Instances map[string]string `json:"instances"`
	UPnPTypes []string          `json:"upnpTypes"`
	Maker     string            `json:"maker"`
	Model     string            `json:"model"`
}

// DeviceDetails returns what discovery learned about a device.
func (m *Monitor) DeviceDetails(id int64) (Details, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.byID[id]
	if e == nil {
		return Details{}, false
	}
	d := Details{Names: map[string]string{}, Instances: map[string]string{}, Maker: e.upnpMaker, Model: e.model,
		Services: append([]string{}, e.services...), UPnPTypes: append([]string{}, e.upnpTypes...)}
	for k, v := range e.names {
		if v != "" && k != "mdns_instance" {
			d.Names[k] = v
		}
	}
	for k, v := range e.instances {
		d.Instances[k] = v
	}
	return d, true
}

// RouterHost is a device as the router's own host table reports it.
type RouterHost struct {
	MAC    string
	IP     netip.Addr
	Name   string // "" when the router only knows a generated name
	Active bool
	WLAN   bool
}

// ApplyRouterHosts takes names from the router and treats an active Wi-Fi
// association as presence: sleeping phones stay associated while they no
// longer answer ARP. Devices are never created from the router's table,
// which also lists hosts that left long ago.
func (m *Monitor) ApplyRouterHosts(ctx context.Context, hosts []RouterHost, at int64) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	matched := 0
	for _, h := range hosts {
		e := m.lookup(h.MAC, h.IP)
		if e == nil {
			continue
		}
		matched++
		changed := false
		if name := textutil.Clean(h.Name, 64); name != "" && e.names["fritzbox"] != name {
			e.names["fritzbox"] = name
			changed = true
		}
		if h.Active && h.WLAN && !e.d.IsSelf {
			if at > e.d.LastSeen {
				e.d.LastSeen = at
				m.dirty[e.d.ID] = at
			}
			if !e.d.Online {
				m.setOnline(ctx, e, at)
				changed = true
			}
		}
		if changed {
			m.reclassify(e)
			m.persist(ctx, e)
			m.publish(e)
		}
	}
	return matched
}
