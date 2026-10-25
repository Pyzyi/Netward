package notify

import "github.com/Pyzyi/netzwart/internal/store"

// Event kinds that can trigger notifications. Channels subscribe to a subset.
const (
	KindDeviceNew     = "device_new"
	KindDeviceOnline  = "device_online"
	KindDeviceOffline = "device_offline"
	KindIPConflict    = "ip_conflict"
	KindInternetDown  = "internet_down"
	KindInternetUp    = "internet_up"
	KindSecurity      = "security_finding"
	KindDeviceFlood   = "device_flood"
	KindTest          = "test"
)

// Subscribable lists the kinds a channel can subscribe to.
var Subscribable = []string{KindDeviceNew, KindDeviceOnline, KindDeviceOffline, KindIPConflict,
	KindInternetDown, KindInternetUp, KindSecurity, KindDeviceFlood}

// Event is something worth telling the user about.
type Event struct {
	Kind     string
	Severity string // info | warning | critical
	TS       int64  // Unix ms
	Device   *store.Device
	Data     map[string]any
}

// Notifier accepts events; implementations must not block.
type Notifier interface {
	Notify(Event)
}

// Discard is a Notifier that drops everything.
type Discard struct{}

// Notify implements Notifier.
func (Discard) Notify(Event) {}
