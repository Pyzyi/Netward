package app

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"sort"
	"time"

	"github.com/Pyzyi/netzwart/internal/fritzbox"
	"github.com/Pyzyi/netzwart/internal/monitor"
	"github.com/Pyzyi/netzwart/internal/scanner"
	"github.com/Pyzyi/netzwart/internal/store"
)

type demoDevice struct {
	mac, ip, host, vendor, model, typ, os string
	name, location                        string
	known, fav, watch, online, gateway    bool
	self, newDev, phone                   bool
	macKind                               string
	services                              []string
	ports                                 []int
	software                              []store.Software // DeviceID and SeenAt are filled in
}

func sw(source, product, version, evidence string) store.Software {
	return store.Software{Source: source, Product: product, Version: version, Evidence: evidence}
}

// demoDevices describe a typical German home network behind a FRITZ!Box.
var demoDevices = []demoDevice{
	{mac: "3c:a6:2f:5e:11:01", ip: "192.168.178.1", host: "fritz.box", vendor: "AVM", model: "FRITZ!Box 7590 AX", typ: "router", name: "FRITZ!Box", location: "Flur", known: true, online: true, gateway: true, ports: []int{53, 80, 443, 49000},
		software: []store.Software{{Source: "fritzbox", Product: "fritzos", Version: "8.20", Evidence: "SoftwareVersion=279.08.20 Update=279.08.25", UpdateAvailable: true}}},
	{mac: "3c:a6:2f:5e:22:02", ip: "192.168.178.2", host: "fritz-repeater", vendor: "AVM", model: "FRITZ!Repeater 2400", typ: "access_point", location: "Obergeschoss", known: true, online: true, ports: []int{80, 443}},
	{mac: "00:e0:4c:68:10:14", ip: "192.168.178.20", host: "zimacube", vendor: "Realtek", typ: "nas", name: "ZimaCube", location: "Arbeitszimmer", known: true, fav: true, online: true, self: true, ports: []int{22, 80, 445}},
	{mac: "00:11:32:a4:7b:20", ip: "192.168.178.21", host: "DiskStation", vendor: "Synology", model: "DS920+", typ: "nas", location: "Arbeitszimmer", known: true, watch: true, online: true, services: []string{"_smb._tcp", "_afpovertcp._tcp", "_http._tcp"}, ports: []int{22, 80, 443, 445, 5000, 5001},
		software: []store.Software{sw("synology", "dsm", "7.2 (Build 72806)", "version_major=7 version_minor=2 version_build=72806")}},
	{mac: "6e:41:0a:93:21:5c", ip: "192.168.178.34", host: "Max-iPhone", vendor: "", model: "iPhone16,1", typ: "phone", os: "iOS", name: "Max' iPhone", known: true, fav: true, watch: true, online: true, phone: true, macKind: "random", services: []string{"_companion-link._tcp", "_apple-mobdev2._tcp"}},
	{mac: "be:22:71:0c:9d:41", ip: "192.168.178.36", host: "Annas-iPhone", vendor: "", model: "iPhone15,2", typ: "phone", os: "iOS", name: "Annas iPhone", known: true, watch: true, online: false, phone: true, macKind: "random", services: []string{"_companion-link._tcp"},
		software: []store.Software{sw("manual", "ios", "26", "")}},
	{mac: "f0:2f:4b:12:7a:33", ip: "192.168.178.41", host: "MacBook-Air", vendor: "Apple", model: "Mac14,2", typ: "laptop", os: "macOS", known: true, online: true, phone: true, services: []string{"_companion-link._tcp", "_ssh._tcp"},
		software: []store.Software{sw("device-info", "macos", "26", "osxvers=25")}},
	{mac: "04:42:1a:c3:55:12", ip: "192.168.178.42", host: "DESKTOP-GAMING", vendor: "ASUS", typ: "computer", os: "Windows", name: "Gaming-PC", location: "Arbeitszimmer", known: true, online: true, ports: []int{135, 139, 445, 3389},
		software: []store.Software{sw("smb", "windows", "10.0.19045", "NTLM 10.0.19045")}},
	{mac: "30:5a:3a:61:0b:2d", ip: "192.168.178.44", host: "LAPTOP-KINDER", vendor: "Lenovo", typ: "laptop", os: "Windows", name: "Laptop Kinder", location: "Kinderzimmer", known: true, online: true, phone: true, ports: []int{135, 139, 445},
		software: []store.Software{sw("smb", "windows", "10.0.26100", "NTLM 10.0.26100")}},
	{mac: "86:2c:10:5f:a4:93", ip: "192.168.178.45", host: "iPad", vendor: "", model: "iPad7,5", typ: "tablet", os: "iPadOS", name: "Omas iPad", location: "Wohnzimmer", known: true, online: true, phone: true, macKind: "random"},
	{mac: "a8:9c:ed:4f:3e:71", ip: "192.168.178.47", host: "Galaxy-Tab-S9", vendor: "Samsung", typ: "tablet", os: "Android", known: true, online: true, phone: true,
		software: []store.Software{sw("dhcp", "android", "16", "android-dhcp-16")}},
	{mac: "72:19:5d:e0:44:a0", ip: "192.168.178.49", host: "Apple-Watch", vendor: "", model: "Watch6,18", typ: "watch", os: "iOS", known: true, online: false, phone: true, macKind: "random"},
	{mac: "8c:79:f5:21:9a:5b", ip: "192.168.178.52", host: "Samsung-QE55Q80", vendor: "Samsung", model: "QE55Q80C", typ: "tv", name: "Fernseher Wohnzimmer", location: "Wohnzimmer", known: true, online: true, ports: []int{8001, 8002, 9197}},
	{mac: "d0:03:4b:7e:12:c4", ip: "192.168.178.53", host: "Schlafzimmer", vendor: "Apple", model: "AppleTV14,1", typ: "streaming", name: "Apple TV Schlafzimmer", location: "Schlafzimmer", known: true, online: true, services: []string{"_airplay._tcp", "_raop._tcp"},
		software: []store.Software{sw("airplay", "tvos", "26.4", "osvers=26.4 model=AppleTV14,1")}},
	{mac: "48:a6:b8:aa:31:10", ip: "192.168.178.55", host: "Sonos-Kueche", vendor: "Sonos", model: "Sonos One", typ: "speaker", name: "Küche", location: "Küche", known: true, online: true, ports: []int{1400},
		software: []store.Software{sw("ssdp", "sonos", "85.0", "Linux UPnP/1.0 Sonos/85.0-66270 (ZPS27)")}},
	{mac: "48:a6:b8:aa:31:44", ip: "192.168.178.56", host: "Sonos-Arc", vendor: "Sonos", model: "Sonos Arc", typ: "speaker", name: "Wohnzimmer Arc", location: "Wohnzimmer", known: true, online: true, ports: []int{1400}},
	{mac: "44:00:49:ab:cc:02", ip: "192.168.178.57", host: "echo-dot", vendor: "Amazon", typ: "speaker", name: "Echo Dot Bad", location: "Bad", known: true, online: true},
	{mac: "c8:d9:d2:3f:61:aa", ip: "192.168.178.60", host: "HP-LaserJet-M234", vendor: "HP", model: "LaserJet MFP M234", typ: "printer", name: "Drucker", location: "Arbeitszimmer", known: true, online: true, services: []string{"_ipp._tcp", "_pdl-datastream._tcp"}, ports: []int{80, 443, 631, 9100}},
	{mac: "78:c8:81:4e:90:02", ip: "192.168.178.62", host: "PS5-723", vendor: "Sony PlayStation", typ: "game_console", name: "PlayStation 5", location: "Wohnzimmer", known: true, online: false},
	{mac: "98:b6:e9:4c:21:07", ip: "192.168.178.63", host: "Nintendo-Switch", vendor: "Nintendo", typ: "game_console", known: true, online: false},
	{mac: "00:17:88:6a:2b:91", ip: "192.168.178.70", host: "Philips-hue", vendor: "Philips Hue", model: "BSB002", typ: "smart_home", name: "Hue Bridge", location: "Wohnzimmer", known: true, online: true, services: []string{"_hue._tcp", "_hap._tcp"}},
	{mac: "c4:5b:be:61:7e:20", ip: "192.168.178.71", host: "shellyplug-s-7E20", vendor: "Espressif", typ: "plug", name: "Steckdose Waschmaschine", location: "Keller", known: true, online: true,
		software: []store.Software{sw("shelly", "shelly", "1.4.4", "ver=1.4.4")}},
	{mac: "c4:5b:be:61:7e:8a", ip: "192.168.178.72", host: "shellyplug-s-7E8A", vendor: "Espressif", typ: "plug", name: "Steckdose Kaffeemaschine", location: "Küche", known: true, online: true},
	{mac: "ec:71:db:12:40:3e", ip: "192.168.178.75", host: "Reolink-Einfahrt", vendor: "Reolink", model: "RLC-810A", typ: "camera", name: "Kamera Einfahrt", location: "Außen", known: true, watch: true, online: true, ports: []int{80, 443, 554, 9000}},
	{mac: "a0:14:3d:22:99:0f", ip: "192.168.178.76", host: "tado-bridge", vendor: "tado°", typ: "smart_home", name: "tado° Bridge", location: "Flur", known: true, online: true},
	{mac: "dc:a6:32:91:0e:5b", ip: "192.168.178.80", host: "pihole", vendor: "Raspberry Pi", typ: "server", name: "Pi-hole", location: "Arbeitszimmer", known: true, online: true, ports: []int{22, 53, 80},
		software: []store.Software{sw("ssh", "debian", "11", "SSH-2.0-OpenSSH_8.4p1 Debian-5+deb11u3")}},
	{mac: "dc:a6:32:7a:3c:18", ip: "192.168.178.81", host: "homeassistant", vendor: "Raspberry Pi", typ: "smart_home", name: "Home Assistant", location: "Arbeitszimmer", known: true, fav: true, online: true, services: []string{"_home-assistant._tcp"}, ports: []int{22, 1883, 8123},
		software: []store.Software{sw("ssh", "debian", "12", "SSH-2.0-OpenSSH_9.2p1 Debian-2+deb12u7"), sw("home-assistant", "homeassistant", "2026.10.2", "version=2026.10.2")}},
	{mac: "50:ec:50:55:a1:2c", ip: "192.168.178.85", host: "roborock-vacuum-s7", vendor: "Roborock", typ: "iot", name: "Saugroboter", location: "Erdgeschoss", known: true, online: true},
	{mac: "24:0a:c4:9e:3b:d1", ip: "192.168.178.90", host: "ESP_9E3BD1", vendor: "Espressif", typ: "iot", known: false, online: true, newDev: true, ports: []int{23, 80}},
	{mac: "5a:8e:13:c0:77:e2", ip: "192.168.178.93", host: "", vendor: "", typ: "phone", known: false, online: true, newDev: true, phone: true, macKind: "random"},
}

// SeedDemo fills an empty database with a realistic sample network. It is
// only used with NETZWART_DEMO=1 (development and screenshots).
func SeedDemo(ctx context.Context, st *store.Store) error {
	existing, err := st.ListDevices(ctx)
	if err != nil || len(existing) > 0 {
		return err
	}
	rnd := rand.New(rand.NewPCG(7, 11))
	var events []store.Event
	now := time.Now()
	baseline := now.Add(-30 * 24 * time.Hour)
	if err := st.SetSetting(ctx, "baseline_at", baseline.UnixMilli()); err != nil {
		return err
	}
	var piholeID int64
	for i, dd := range demoDevices {
		first := baseline.Add(-time.Duration(rnd.IntN(200)) * 24 * time.Hour)
		if dd.newDev {
			first = now.Add(-time.Duration(20+i*7) * time.Minute)
		}
		last := now
		if !dd.online {
			last = now.Add(-time.Duration(30+rnd.IntN(600)) * time.Minute)
		}
		kind := dd.macKind
		if kind == "" {
			kind = "universal"
		}
		d := store.Device{Ident: dd.mac, MAC: dd.mac, IP: dd.ip, Hostname: dd.host, Vendor: dd.vendor, VendorOfficial: dd.vendor,
			Model: dd.model, OS: dd.os, DetectedType: dd.typ, TypeConfidence: 80, MACKind: kind, Online: dd.online,
			IsGateway: dd.gateway, IsSelf: dd.self, Interface: "eth0", FirstSeen: first.UnixMilli(), LastSeen: last.UnixMilli()}
		if err := st.InsertDevice(ctx, &d); err != nil {
			return err
		}
		name, loc := dd.name, dd.location
		known, fav, watch := dd.known, dd.fav, dd.watch
		if err := st.PatchDevice(ctx, d.ID, store.DevicePatch{Name: &name, Location: &loc, Known: &known, Favorite: &fav, Watch: &watch}); err != nil {
			return err
		}
		if dd.host != "" {
			_ = st.UpsertName(ctx, d.ID, "dhcp", dd.host)
		}
		var svcs []store.Service
		for _, s := range dd.services {
			svcs = append(svcs, store.Service{Source: "mdns", Service: s})
		}
		_ = st.ReplaceServices(ctx, d.ID, "mdns", svcs)
		var ports []store.Port
		for _, p := range dd.ports {
			ports = append(ports, store.Port{Port: p, Proto: "tcp", Service: scanner.Services[p]})
		}
		if len(ports) > 0 {
			_ = st.ReplacePorts(ctx, d.ID, ports, now.Add(-2*time.Hour).UnixMilli())
		}
		_ = st.TouchAddress(ctx, d.ID, dd.ip, "eth0", last.UnixMilli())
		for _, s := range dd.software {
			s.DeviceID, s.SeenAt = d.ID, now.Add(-3*time.Hour).UnixMilli()
			_ = st.UpsertSoftware(ctx, s)
		}
		// Presence over the last 7 days: mobile devices come and go, the rest stays.
		cursor := now.Add(-7 * 24 * time.Hour)
		if first.After(cursor) {
			cursor = first
		}
		for cursor.Before(last) {
			var on, off time.Duration
			if dd.phone {
				on = time.Duration(2+rnd.IntN(10)) * time.Hour
				off = time.Duration(1+rnd.IntN(9)) * time.Hour
			} else {
				on = time.Duration(40+rnd.IntN(120)) * time.Hour
				off = time.Duration(5+rnd.IntN(40)) * time.Minute
			}
			end := cursor.Add(on)
			_ = st.OpenPresence(ctx, d.ID, cursor.UnixMilli())
			if end.After(last) {
				if !dd.online {
					_ = st.ClosePresence(ctx, d.ID, last.UnixMilli())
				}
				break
			}
			_ = st.ClosePresence(ctx, d.ID, end.UnixMilli())
			cursor = end.Add(off)
		}
		if dd.newDev {
			data, _ := json.Marshal(map[string]any{"name": firstNonEmpty(dd.host, dd.vendor, dd.mac), "ip": dd.ip, "vendor": dd.vendor, "deviceType": dd.typ})
			events = append(events, store.Event{TS: first.UnixMilli(), Kind: "device_new", DeviceID: d.ID, Data: data})
		}
		if dd.watch && !dd.online {
			data, _ := json.Marshal(map[string]any{"name": dd.name, "ip": dd.ip, "deviceType": dd.typ})
			events = append(events, store.Event{TS: last.UnixMilli(), Kind: "device_offline", DeviceID: d.ID, Data: data})
		}
		if dd.watch && dd.phone && dd.online {
			data, _ := json.Marshal(map[string]any{"name": dd.name, "ip": dd.ip, "deviceType": dd.typ})
			events = append(events, store.Event{TS: now.Add(-47 * time.Minute).UnixMilli(), Kind: "device_online", DeviceID: d.ID, Data: data})
		}
		if dd.mac == "24:0a:c4:9e:3b:d1" {
			fd, _ := json.Marshal(map[string]any{"port": 23, "service": "telnet"})
			_, _ = st.UpsertFinding(ctx, store.Finding{CheckID: "risky_port", DeviceID: d.ID, Severity: "critical", Data: fd, LastSeen: now.UnixMilli()})
		}
		if dd.gateway {
			_, _ = st.UpsertFinding(ctx, store.Finding{CheckID: "upnp_igd", DeviceID: d.ID, Severity: "warning", Data: json.RawMessage("{}"), LastSeen: now.UnixMilli()})
		}
		if dd.mac == "dc:a6:32:91:0e:5b" {
			piholeID = d.ID
		}
		if dd.mac == "04:42:1a:c3:55:12" {
			fd, _ := json.Marshal(map[string]any{"port": 3389, "service": "rdp"})
			_, _ = st.UpsertFinding(ctx, store.Finding{CheckID: "risky_port", DeviceID: d.ID, Severity: "warning", Data: fd, LastSeen: now.UnixMilli()})
		}
	}
	// The Pi-hole hands out addresses too, next to the FRITZ!Box: the classic
	// "DHCP switched on in both places" mistake (docs/09, F1).
	seen := now.Add(-2 * time.Hour).UnixMilli()
	_ = st.SetSetting(ctx, "net_dhcp_servers", []monitor.DHCPServer{
		{ServerID: "192.168.178.1", MAC: "3c:a6:2f:5e:11:01", Vendor: "AVM", Router: "192.168.178.1", DNS: []string{"192.168.178.80"},
			FirstSeen: baseline.UnixMilli(), LastSeen: seen, Accepted: true},
		{ServerID: "192.168.178.80", MAC: "dc:a6:32:91:0e:5b", Vendor: "Raspberry Pi", Router: "192.168.178.1", DNS: []string{"192.168.178.80"},
			FirstSeen: now.Add(-26 * time.Hour).UnixMilli(), LastSeen: seen},
	})
	_ = st.SetSetting(ctx, "net_dhcp_probe", monitor.DHCPProbe{At: now.Add(-3 * time.Hour).UnixMilli(), Answers: 2})
	_ = st.SetSetting(ctx, "net_gateway_refs", []monitor.GatewayRef{
		{IP: "192.168.178.1", MAC: "3c:a6:2f:5e:11:01", Vendor: "AVM", Since: baseline.UnixMilli()},
	})
	dsrv, _ := json.Marshal(map[string]any{"ip": "192.168.178.80", "vendor": "Raspberry Pi"})
	events = append(events, store.Event{TS: now.Add(-26 * time.Hour).UnixMilli(), Kind: "dhcp_server_new", Data: dsrv})
	_, _ = st.UpsertFinding(ctx, store.Finding{CheckID: "dhcp_rogue", DeviceID: piholeID, Severity: "warning",
		Data: json.RawMessage(`{"server":"192.168.178.80"}`), LastSeen: now.UnixMilli()})
	_, _ = st.UpsertFinding(ctx, store.Finding{CheckID: "unknown_devices", Severity: "info", Data: json.RawMessage("{}"), LastSeen: now.UnixMilli()})
	_ = st.SetSetting(ctx, "security_last_run", now.Add(-3*time.Hour).UnixMilli())
	out := now.Add(-20 * time.Hour)
	if _, err := st.OpenOutage(ctx, out.UnixMilli(), store.OutageInternet); err == nil {
		_ = st.CloseOutages(ctx, out.Add(23*time.Minute).UnixMilli())
	}
	if err := seedLineHistory(ctx, st, now, rnd); err != nil {
		return err
	}
	data, _ := json.Marshal(map[string]any{"durationMs": int64(23 * time.Minute / time.Millisecond)})
	events = append(events, store.Event{TS: out.UnixMilli(), Kind: "internet_down", Severity: "warning", Data: json.RawMessage(`{"state":"offline"}`)})
	events = append(events, store.Event{TS: out.Add(23 * time.Minute).UnixMilli(), Kind: "internet_up", Data: data})
	ipc, _ := json.Marshal(map[string]any{"name": "Galaxy-Tab-S9", "from": "192.168.178.44", "to": "192.168.178.47", "deviceType": "tablet"})
	events = append(events, store.Event{TS: now.Add(-5 * time.Hour).UnixMilli(), Kind: "ip_changed", Data: ipc})
	// Insert in time order so IDs follow time like in real operation.
	sort.Slice(events, func(i, j int) bool { return events[i].TS < events[j].TS })
	for i := range events {
		if err := st.AddEvent(ctx, &events[i]); err != nil {
			return err
		}
	}
	return nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// DemoWLAN is the Wi-Fi of the demo home: the FRITZ!Box downstairs and a
// repeater upstairs whose own link is weak, so the children's laptop ends
// up on 2.4 GHz behind it (docs/09, F3).
func DemoWLAN() fritzbox.WLANState {
	const box, rep = "3c:a6:2f:5e:11:01", "3c:a6:2f:5e:22:02"
	c := func(mac, ap, band string, ch, dbm, rate, maxRate int, can5 bool) fritzbox.WLANClient {
		return fritzbox.WLANClient{MAC: mac, APMAC: ap, Band: band, Channel: ch, SignalDBm: dbm, RateRx: rate, RateTx: rate * 9 / 10, MaxRate: maxRate, Can5GHz: can5}
	}
	return fritzbox.WLANState{
		Source: "mesh",
		APs: []fritzbox.WLANAccessPoint{
			{MAC: box, Name: "FRITZ!Box", Model: "FRITZ!Box 7590 AX", Master: true, Bands: []string{"2.4", "5"}},
			{MAC: rep, Name: "Repeater OG", Model: "FRITZ!Repeater 2400", Bands: []string{"2.4", "5"},
				Backhaul: &fritzbox.WLANBackhaul{Type: "wlan", Band: "5", SignalDBm: -77, Rate: 86}},
		},
		Clients: []fritzbox.WLANClient{
			c("6e:41:0a:93:21:5c", box, "5", 36, -54, 864, 1201, true),
			c("f0:2f:4b:12:7a:33", box, "5", 36, -61, 702, 1201, true),
			c("8c:79:f5:21:9a:5b", box, "5", 36, -57, 433, 866, true),
			c("a8:9c:ed:4f:3e:71", box, "2.4", 6, -70, 72, 286, true),
			c("44:00:49:ab:cc:02", box, "2.4", 6, -68, 65, 72, false),
			c("48:a6:b8:aa:31:10", box, "2.4", 6, -55, 72, 72, false),
			c("ec:71:db:12:40:3e", box, "2.4", 6, -73, 58, 144, false),
			c("c4:5b:be:61:7e:20", box, "2.4", 6, -84, 6, 72, false),
			c("c4:5b:be:61:7e:8a", box, "2.4", 6, -66, 39, 72, false),
			c("50:ec:50:55:a1:2c", box, "2.4", 6, -62, 52, 72, false),
			c("5a:8e:13:c0:77:e2", box, "5", 36, -66, 390, 866, true),
			c("30:5a:3a:61:0b:2d", rep, "2.4", 11, -79, 26, 144, true),
			c("86:2c:10:5f:a4:93", rep, "5", 36, -66, 270, 866, true),
		},
	}
}

// seedLineHistory fills 30 days of latency and a handful of outages of each
// kind, so the provider report (docs/09, F5) has something to show.
func seedLineHistory(ctx context.Context, st *store.Store, now time.Time, rnd *rand.Rand) error {
	type spell struct {
		daysAgo, hour, minutes int
		kind                   string
	}
	spells := []spell{
		{27, 3, 4, store.OutageInternet}, {21, 20, 41, store.OutageInternet}, {14, 2, 2, store.OutageRouter},
		{9, 19, 12, store.OutageInternet}, {6, 21, 7, store.OutageDNS}, {3, 1, 3, store.OutageInternet},
	}
	down := func(t time.Time) bool {
		for _, s := range spells {
			start := time.Date(now.Year(), now.Month(), now.Day()-s.daysAgo, s.hour, 10, 0, 0, now.Location())
			if s.kind != store.OutageRouter && !t.Before(start) && t.Before(start.Add(time.Duration(s.minutes)*time.Minute)) {
				return true
			}
		}
		return false
	}
	for _, s := range spells {
		start := time.Date(now.Year(), now.Month(), now.Day()-s.daysAgo, s.hour, 10, 0, 0, now.Location())
		if _, err := st.OpenOutage(ctx, start.UnixMilli(), s.kind); err != nil {
			return err
		}
		if s.kind == store.OutageDNS {
			_ = st.CloseDNSOutages(ctx, start.Add(time.Duration(s.minutes)*time.Minute).UnixMilli())
		} else {
			_ = st.CloseOutages(ctx, start.Add(time.Duration(s.minutes)*time.Minute).UnixMilli())
		}
	}
	var ls []store.Latency
	for t := now.Add(-30 * 24 * time.Hour).Truncate(5 * time.Minute); t.Before(now.Add(-5 * time.Minute)); t = t.Add(5 * time.Minute) {
		med := 11 + rnd.IntN(4)
		if h := t.Hour(); h >= 19 && h <= 22 {
			med += 6 + rnd.IntN(8) // evening peak
		}
		l := store.Latency{TS: t.UnixMilli(), Median: med, Max: med + 3 + rnd.IntN(25), Samples: 10}
		if down(t) {
			l.Median, l.Max, l.Lost = -1, -1, 10
		} else if rnd.IntN(40) == 0 {
			l.Lost = 1
		}
		ls = append(ls, l)
	}
	return st.AddLatency(ctx, ls...)
}
