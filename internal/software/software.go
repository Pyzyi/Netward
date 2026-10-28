// Package software turns what devices reveal about themselves (mDNS TXT
// records, SSDP and DHCP identifiers, SSH and SMB greetings, the device model)
// into a statement the user can act on: which system runs there and whether
// it still gets security updates.
//
// Everything here is local: the knowledge base is compiled in and carries its
// own date (DataAsOf). Parsers are pure functions on bounded input.
package software

import (
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Pyzyi/Netward/internal/vuln"
)

// Sources, in the order of their precision (most precise first).
const (
	SourceManual        = "manual"         // entered by the user
	SourceSMB           = "smb"            // NTLM version in the SMB handshake
	SourceFritzBox      = "fritzbox"       // TR-064, the router about itself
	SourceSSH           = "ssh"            // SSH identification string
	SourceAirPlay       = "airplay"        // mDNS _airplay._tcp osvers=
	SourceDeviceInfo    = "device-info"    // mDNS _device-info._tcp osxvers=
	SourceSynology      = "synology"       // mDNS _http._tcp version_*
	SourceESPHome       = "esphome"        // mDNS _esphomelib._tcp version=
	SourceShelly        = "shelly"         // mDNS _shelly._tcp ver=
	SourceHomeAssistant = "home-assistant" // mDNS _home-assistant._tcp version=
	SourceSSDP          = "ssdp"           // SSDP SERVER header
	SourceDHCP          = "dhcp"           // DHCP option 60
	SourceModel         = "model"          // derived from the model identifier
)

// Active probes and the router's own answer rank above passive announcements,
// which any device on the LAN can forge for another address (T-30).
var sourceRank = map[string]int{
	SourceManual: 0, SourceSMB: 1, SourceFritzBox: 2, SourceSSH: 3, SourceAirPlay: 4, SourceDeviceInfo: 5,
	SourceSynology: 6, SourceHomeAssistant: 7, SourceESPHome: 8, SourceShelly: 9, SourceSSDP: 10, SourceDHCP: 11,
	SourceModel: 12,
}

// Products.
const (
	Windows       = "windows"
	IOS           = "ios"
	IPadOS        = "ipados"
	MacOS         = "macos"
	TvOS          = "tvos"
	AudioOS       = "audioos"
	Android       = "android"
	Debian        = "debian"
	Ubuntu        = "ubuntu"
	DSM           = "dsm"
	FritzOS       = "fritzos"
	ESPHome       = "esphome"
	Shelly        = "shelly"
	HomeAssistant = "homeassistant"
	Sonos         = "sonos"
	Linux         = "linux"    // kernel version, shown only
	OpenSSH       = "openssh"  // when no distribution can be told, shown only
	Dropbear      = "dropbear" // embedded SSH server, shown only
	BusyBox       = "busybox"  // embedded DHCP client, shown only
)

// Observation is one statement about the software of a device.
type Observation struct {
	Source   string `json:"source"`
	Product  string `json:"product"`
	Version  string `json:"version"`  // as reported, normalised ("26.1", "26100", "12")
	Evidence string `json:"evidence"` // short raw value, for the user to check
	SeenAt   int64  `json:"seenAt"`
	// UpdateAvailable is set when the device itself reports a pending update.
	UpdateAvailable bool `json:"updateAvailable,omitempty"`
}

// Status of an assessment.
const (
	StatusSupported = "supported" // gets security updates
	StatusAging     = "aging"     // still updated, end in sight (older branch, cannot upgrade further)
	StatusOutdated  = "outdated"  // no security updates any more
	StatusUpdate    = "update"    // the device reports an update waiting to be installed
	StatusUnknown   = "unknown"
)

// Assessment is what the UI shows for a device.
type Assessment struct {
	Status   string `json:"status"`
	Product  string `json:"product"`            // "windows", "ios", ...
	Name     string `json:"name"`               // "Windows 11 24H2", "iOS 18", "Debian 12"
	Version  string `json:"version"`            // detail ("Build 26100", "osvers=18.2")
	Since    string `json:"since,omitempty"`    // YYYY-MM-DD: no updates since / support ends on
	Until    string `json:"until,omitempty"`    // YYYY-MM-DD: supported until (if known)
	MaxOS    string `json:"maxOs,omitempty"`    // "iOS 16": highest system the model can run
	Note     string `json:"note,omitempty"`     // reason code for an extra explanation in the UI
	Edition  string `json:"edition,omitempty"`  // Windows edition chosen by the user
	Conflict bool   `json:"conflict,omitempty"` // sources name different systems
	LastSeen int64  `json:"lastSeen,omitempty"` // with NoteStale: when the system was last seen
	Source   string `json:"source,omitempty"`   // where the deciding observation came from
	Evidence string `json:"evidence,omitempty"` // its raw value
	// Observations lists every source, most precise first.
	Observations []Observation `json:"observations"`
	DataAsOf     string        `json:"dataAsOf"`
	// Exploited is the match against known exploited vulnerabilities
	// (docs/09, F6); nil when the version is too vague to tell.
	Exploited *Exploited `json:"exploited,omitempty"`
}

// Exploited is what the offline snapshot says about the deciding version.
type Exploited struct {
	vuln.Result
	Version  string `json:"version"`
	Manual   bool   `json:"manual,omitempty"` // the version was entered by the user
	DataAsOf string `json:"dataAsOf"`
}

// exploited matches an observation; manual entries count for StaleAfter
// like everything else, since the device may have been updated meanwhile.
func exploited(o Observation, now time.Time) *Exploited {
	if !slices.Contains(vuln.Products, o.Product) {
		return nil
	}
	if o.Source == SourceManual && o.SeenAt > 0 && now.Sub(time.UnixMilli(o.SeenAt)) > StaleAfter {
		return nil
	}
	r := vuln.Check(o.Product, o.Version)
	if r.Verdict == vuln.Unknown {
		return nil
	}
	return &Exploited{Result: r, Version: o.Version, Manual: o.Source == SourceManual, DataAsOf: vuln.DataAsOf()}
}

// Sort orders observations by precision, then by recency.
func Sort(obs []Observation) {
	sort.SliceStable(obs, func(i, j int) bool {
		ri, rj := rank(obs[i].Source), rank(obs[j].Source)
		if ri != rj {
			return ri < rj
		}
		return obs[i].SeenAt > obs[j].SeenAt
	})
}

func rank(s string) int {
	if r, ok := sourceRank[s]; ok {
		return r
	}
	return len(sourceRank)
}

// StaleAfter: an observation that has not been repeated for this long no
// longer decides the status; the UI shows when it was last seen instead.
const StaleAfter = 90 * 24 * time.Hour

// Windows editions with longer servicing than Home/Pro (the default, "").
const (
	EditionEnterprise = "enterprise" // Enterprise, Education
	EditionLTSC       = "ltsc"       // LTSC/LTSB, Server
)

// Input is everything the assessment looks at.
type Input struct {
	Model        string // Apple model identifier or marketing model
	Observations []Observation
	Edition      string // "" (Home/Pro), EditionEnterprise or EditionLTSC
	Now          time.Time
}

// Assess decides the status of a device. The most precise fresh observation
// the knowledge base can judge wins; observations it cannot judge (ESPHome,
// kernel versions) are still listed.
func Assess(in Input) Assessment {
	obs := append([]Observation(nil), in.Observations...)
	Sort(obs)
	if obs == nil {
		obs = []Observation{}
	}
	out := Assessment{Status: StatusUnknown, Observations: obs, DataAsOf: DataAsOf, Edition: in.Edition}
	stale := func(o Observation) bool {
		return o.Source != SourceManual && o.SeenAt > 0 && in.Now.Sub(time.UnixMilli(o.SeenAt)) > StaleAfter
	}
	finish := func(a Assessment, o Observation) Assessment {
		a.Observations, a.DataAsOf, a.Edition = obs, DataAsOf, in.Edition
		a.Source, a.Evidence = o.Source, o.Evidence
		if mx, ok := appleMax(in.Model); ok {
			a.MaxOS = mx.label()
		}
		a.Conflict = conflicting(obs, o, in.Model, stale)
		return a
	}
	byModel := func() (Assessment, bool) {
		mx, ok := appleMax(in.Model)
		if !ok {
			return Assessment{}, false
		}
		a := judgeAppleMax(mx)
		a.Observations, a.DataAsOf, a.Edition = obs, DataAsOf, in.Edition
		a.Source, a.Evidence = SourceModel, clip(in.Model, 40)
		return a, true
	}
	for _, o := range obs {
		if stale(o) {
			continue
		}
		a, ok := judge(o, in.Now, in.Edition)
		if !ok {
			continue
		}
		// A version above what the model can install cannot be right (a
		// spoofed announcement for this address): the model decides.
		if mx, ok := appleMax(in.Model); ok && family(o.Product) == "apple" && o.Source != SourceManual {
			if m, ok := major(o.Version); ok && m > mx.major {
				if b, ok := byModel(); ok {
					b.Conflict = true
					return expire(b, in.Now)
				}
			}
		}
		a = finish(a, o)
		a.Exploited = exploited(o, in.Now)
		return expire(a, in.Now)
	}
	// Only old observations: the model may still decide; otherwise name what
	// was seen, but decide nothing.
	for _, o := range obs {
		if a, ok := judge(o, in.Now, in.Edition); ok {
			if b, ok := byModel(); ok && b.Status != StatusUnknown {
				b.LastSeen = o.SeenAt
				return b
			}
			a = finish(a, o)
			a.Status, a.Note, a.Since, a.Until, a.LastSeen = StatusUnknown, NoteStale, "", "", o.SeenAt
			return a
		}
	}
	// No version: the model alone may still say "cannot be updated any more".
	if a, ok := byModel(); ok {
		return a
	}
	// Something without a verdict (e.g. ESPHome): show it, status unknown.
	if len(obs) > 0 {
		out.Product, out.Name, out.Version = obs[0].Product, displayName(obs[0]), obs[0].Version
		out.Source, out.Evidence = obs[0].Source, obs[0].Evidence
	}
	return out
}

// expire turns a "supported until" that has passed into "outdated since".
func expire(a Assessment, now time.Time) Assessment {
	if (a.Status == StatusSupported || a.Status == StatusAging) && a.Until != "" && now.After(mustDate(a.Until)) {
		a.Status, a.Since, a.Until = StatusOutdated, a.Until, ""
	}
	return a
}

// family groups products that cannot run on the same device at once.
func family(product string) string {
	switch product {
	case Windows:
		return "windows"
	case IOS, IPadOS, MacOS, TvOS, AudioOS:
		return "apple"
	case Android:
		return "android"
	case Debian, Ubuntu:
		return "linux"
	case DSM, FritzOS:
		return product
	}
	return ""
}

// conflicting reports whether another fresh source names a different system
// than the deciding one, e.g. a spoofed announcement for this address.
func conflicting(obs []Observation, decided Observation, model string, stale func(Observation) bool) bool {
	f := family(decided.Product)
	if f == "" {
		return false
	}
	for _, o := range obs {
		if o == decided || stale(o) {
			continue
		}
		if g := family(o.Product); g != "" && g != f {
			return true
		}
	}
	if _, ok := appleMax(model); ok && f != "apple" {
		return true
	}
	return false
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}
