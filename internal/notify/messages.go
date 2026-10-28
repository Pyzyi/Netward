package notify

import (
	"fmt"
	"strings"
	"time"

	"github.com/Pyzyi/Netward/internal/store"
)

// Message is a rendered, channel-independent notification. Title and Body
// are plain text; every channel encodes them for its own format.
type Message struct {
	Kind     string
	Severity string
	Title    string
	Body     string
	TS       int64
	Device   *store.Device
	Data     map[string]any
}

var typeNames = map[string][2]string{
	"router": {"Router", "Router"}, "access_point": {"Access Point", "Access point"}, "switch": {"Switch", "Switch"},
	"server": {"Server", "Server"}, "nas": {"NAS", "NAS"}, "computer": {"Computer", "Computer"},
	"laptop": {"Laptop", "Laptop"}, "phone": {"Smartphone", "Phone"}, "tablet": {"Tablet", "Tablet"},
	"watch": {"Smartwatch", "Watch"}, "tv": {"Fernseher", "TV"}, "streaming": {"Streaming-Gerät", "Streaming device"},
	"speaker": {"Lautsprecher", "Speaker"}, "game_console": {"Spielkonsole", "Game console"},
	"printer": {"Drucker", "Printer"}, "camera": {"Kamera", "Camera"}, "smart_home": {"Smart Home", "Smart home"},
	"light": {"Licht", "Light"}, "plug": {"Steckdose", "Plug"}, "iot": {"IoT-Gerät", "IoT device"},
	"virtual": {"Virtuelle Maschine", "Virtual machine"}, "unknown": {"Gerät", "Device"},
}

// TypeName returns the localized label of a device type.
func TypeName(t, lang string) string {
	n, ok := typeNames[t]
	if !ok {
		n = typeNames["unknown"]
	}
	if lang == "en" {
		return n[1]
	}
	return n[0]
}

// DeviceName returns the name shown in notifications.
func DeviceName(d *store.Device, lang string) string {
	if d == nil {
		return ""
	}
	switch {
	case d.Name != "":
		return d.Name
	case d.Hostname != "":
		return d.Hostname
	case d.Vendor != "":
		if lang == "en" {
			return d.Vendor + " " + strings.ToLower(TypeName(d.Type, lang))
		}
		return d.Vendor + "-" + TypeName(d.Type, lang)
	}
	return TypeName(d.Type, lang) + " " + d.MAC
}

func tr(lang, de, en string) string {
	if lang == "en" {
		return en
	}
	return de
}

// Duration formats d compactly ("2 h 5 min").
func Duration(d time.Duration, lang string) string {
	d = d.Round(time.Minute)
	if d < time.Minute {
		return tr(lang, "weniger als 1 min", "less than 1 min")
	}
	h, m := int(d.Hours()), int(d.Minutes())%60
	switch {
	case h == 0:
		return fmt.Sprintf("%d min", m)
	case m == 0:
		return fmt.Sprintf("%d h", h)
	}
	return fmt.Sprintf("%d h %d min", h, m)
}

func str(m map[string]any, k string) string {
	if v, ok := m[k]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

// Render turns an event into a message in the given language and time zone.
func Render(ev Event, lang string, loc *time.Location) Message {
	if loc == nil {
		loc = time.UTC
	}
	msg := Message{Kind: ev.Kind, Severity: ev.Severity, TS: ev.TS, Device: ev.Device, Data: ev.Data}
	name := DeviceName(ev.Device, lang)
	when := time.UnixMilli(ev.TS).In(loc).Format("02.01.2006 15:04")
	if lang == "en" {
		when = time.UnixMilli(ev.TS).In(loc).Format("2006-01-02 15:04")
	}
	details := func() string {
		d := ev.Device
		if d == nil {
			return ""
		}
		var parts []string
		if d.Vendor != "" {
			parts = append(parts, d.Vendor)
		}
		if d.IP != "" {
			parts = append(parts, d.IP)
		}
		parts = append(parts, d.MAC)
		s := strings.Join(parts, " · ")
		if d.MACKind == "random" {
			s += tr(lang, " (private MAC-Adresse)", " (private MAC address)")
		}
		return s
	}
	switch ev.Kind {
	case KindDeviceNew:
		msg.Title = tr(lang, "Neues Gerät im Netz", "New device on your network")
		msg.Body = name + "\n" + details()
	case KindDeviceOnline:
		msg.Title = name + tr(lang, " ist online", " is online")
		msg.Body = details()
	case KindDeviceOffline:
		msg.Title = name + tr(lang, " ist offline", " is offline")
		msg.Body = details()
	case KindIPConflict:
		ip := str(ev.Data, "ip")
		msg.Title = tr(lang, "IP-Konflikt bei ", "IP address conflict on ") + ip
		if g, _ := ev.Data["gateway"].(bool); g {
			msg.Title = tr(lang, "Zwei Geräte beanspruchen die Router-Adresse ", "Two devices claim the router address ") + ip
		}
		msg.Body = tr(lang, "Mehrere Geräte antworten auf diese Adresse: ", "Several devices answer for this address: ") +
			strings.Trim(str(ev.Data, "macs"), "[]")
	case KindInternetDown:
		msg.Title = tr(lang, "Internetverbindung unterbrochen", "Internet connection lost")
		msg.Body = tr(lang, "Seit ", "Since ") + when
	case KindInternetUp:
		msg.Title = tr(lang, "Internet wieder verfügbar", "Internet connection restored")
		if ms, ok := ev.Data["durationMs"].(int64); ok {
			msg.Body = tr(lang, "Unterbrechung: ", "Outage: ") + Duration(time.Duration(ms)*time.Millisecond, lang)
		}
	case KindSecurity:
		msg.Title = tr(lang, "Sicherheitshinweis: ", "Security notice: ") + str(ev.Data, "title")
		msg.Body = strings.TrimSpace(name + "\n" + str(ev.Data, "detail"))
	case KindDeviceFlood:
		msg.Title = tr(lang, "Ungewöhnlich viele neue Geräte", "Unusually many new devices")
		msg.Body = tr(lang, "Netward nimmt vorübergehend keine weiteren neuen Geräte auf. Das kann auf ein fehlerhaftes Gerät oder einen Angriff im Netz hindeuten.",
			"Netward temporarily stopped adding new devices. This can indicate a faulty device or an attack on the network.")
	case KindTest:
		msg.Title = tr(lang, "Testnachricht von Netward", "Test message from Netward")
		msg.Body = tr(lang, "Wenn du das liest, ist der Kanal richtig eingerichtet.", "If you can read this, the channel works.")
	default:
		msg.Title = ev.Kind
	}
	return msg
}

// RenderSummary combines several new-device events into one message.
func RenderSummary(evs []Event, lang string) Message {
	var lines []string
	for i, ev := range evs {
		if i == 10 {
			lines = append(lines, fmt.Sprintf(tr(lang, "… und %d weitere", "… and %d more"), len(evs)-10))
			break
		}
		line := DeviceName(ev.Device, lang)
		if ev.Device != nil && ev.Device.IP != "" {
			line += " (" + ev.Device.IP + ")"
		}
		lines = append(lines, "• "+line)
	}
	return Message{
		Kind: KindDeviceNew, Severity: "info", TS: evs[len(evs)-1].TS,
		Title: fmt.Sprintf(tr(lang, "%d neue Geräte im Netz", "%d new devices on your network"), len(evs)),
		Body:  strings.Join(lines, "\n"),
	}
}
