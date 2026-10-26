package software

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DataAsOf is the date of the knowledge base below. Sources are listed in
// docs/08-software-und-support.md; update both with every release.
const DataAsOf = "2026-10-26"

// Apple: current major release and the previous one, which still gets
// security fixes. iOS/iPadOS 18 is the last branch for devices that cannot
// run 26 and still received fixes in August 2026 (18.7.10).
const (
	appleCurrent  = 27
	applePrevious = 26
	appleLegacy   = 18 // iOS/iPadOS/tvOS branch still patched, end in sight
	macN2         = 15 // macOS: current and two predecessors get fixes
)

// Reason codes for extra explanations in the UI.
const (
	NoteOlderBranch    = "older_branch"     // iOS 18 line: fixes, but no new major release
	NoteCannotUpdate   = "cannot_update"    // model cannot run the current major release
	NoteMacN2          = "macos_n2"         // last macOS that still gets fixes
	NoteWinEditionHint = "win_edition_hint" // Home/Pro ended, another edition of the build is still serviced
	NoteWinEnterprise  = "win_enterprise"   // judged as Enterprise/Education
	NoteWinLTSCSet     = "win_ltsc"         // judged as LTSC/Server
	NoteWinNoLTSC      = "win_no_ltsc"      // LTSC chosen, but this build has none
	NoteWinESU         = "win10_esu"        // Windows 10: also the paid extension has ended
	NoteDebianLTS      = "debian_lts"       // supported by Debian LTS
	NoteAndroidOEM     = "android_oem"      // patches also depend on the manufacturer
	NoteNewerThanKB    = "newer_than_kb"    // newer than the knowledge base: assumed supported
	NoteFritzOld       = "fritz_old"        // FRITZ!OS 6 or older
	NoteUbuntuESM      = "ubuntu_esm"       // only with Ubuntu Pro (ESM)
	NoteUbuntuInterm   = "ubuntu_interim"   // short-lived interim release
	NoteAppleTVUpdate  = "apple_tv_update"  // tvOS/audioOS: only the newest release is patched
	NoteDSMOld         = "dsm_old"          // DSM 7.0/7.1: update to 7.2
	NoteFritzEOM       = "fritz_eom"        // FRITZ!OS 7 without an update: model may be end of maintenance
	NoteStale          = "stale"            // not seen for a long time
)

func judge(o Observation, now time.Time, edition string) (Assessment, bool) {
	a := Assessment{Product: o.Product, Name: displayName(o), Version: o.Version}
	switch o.Product {
	case TvOS, AudioOS:
		// Apple only patches the newest tvOS/audioOS; the previous release
		// means an update is waiting (or the hardware is too old for it).
		m, ok := major(o.Version)
		if !ok {
			return a, false
		}
		switch {
		case m > appleCurrent:
			a.Status, a.Note = StatusSupported, NoteNewerThanKB
		case m == appleCurrent:
			a.Status = StatusSupported
		case m == applePrevious:
			a.Status, a.Note = StatusUpdate, NoteAppleTVUpdate
		default:
			a.Status = StatusOutdated
		}
		return a, true
	case IOS, IPadOS:
		m, ok := major(o.Version)
		if !ok {
			return a, false
		}
		switch {
		case m > appleCurrent:
			a.Status, a.Note = StatusSupported, NoteNewerThanKB
		case m >= applePrevious:
			a.Status = StatusSupported
		case m == appleLegacy:
			a.Status, a.Note = StatusAging, NoteOlderBranch
		default:
			a.Status = StatusOutdated
		}
		return a, true
	case MacOS:
		m, ok := major(o.Version)
		if !ok {
			return a, false
		}
		switch {
		case m > appleCurrent:
			a.Status, a.Note = StatusSupported, NoteNewerThanKB
		case m >= applePrevious:
			a.Status = StatusSupported
		case m == macN2:
			a.Status, a.Note = StatusAging, NoteMacN2
		default:
			a.Status = StatusOutdated
		}
		return a, true
	case Windows:
		return judgeWindows(o.Version, now, edition)
	case Android:
		m, ok := major(o.Version)
		if !ok {
			return a, false
		}
		a.Note = NoteAndroidOEM
		switch {
		case m >= 14:
			a.Status = StatusSupported
		case m == 13:
			a.Status, a.Since = StatusOutdated, "2026-03-02"
		default:
			a.Status = StatusOutdated
		}
		return a, true
	case Debian:
		m, ok := major(o.Version)
		if !ok {
			return a, false
		}
		switch {
		case m >= 13:
			a.Status = StatusSupported
		case m == 12:
			a.Status, a.Until, a.Note = StatusSupported, "2028-06-30", NoteDebianLTS
		case m == 11:
			a.Status, a.Since = StatusOutdated, "2026-08-31"
		case m == 10:
			a.Status, a.Since = StatusOutdated, "2024-06-30"
		default:
			a.Status = StatusOutdated
		}
		return a, true
	case Ubuntu:
		return judgeUbuntu(a, o.Version, now)
	case DSM:
		m, ok := major(o.Version)
		if !ok {
			return a, false
		}
		minor := 0
		if _, rest, ok := strings.Cut(o.Version, "."); ok {
			minor, _ = strconv.Atoi(strings.TrimRightFunc(rest[:min(len(rest), 2)], func(r rune) bool { return r < '0' || r > '9' }))
		}
		if m > 7 || m == 7 && minor >= 2 {
			a.Status = StatusSupported
		} else if m == 7 {
			a.Status, a.Note = StatusAging, NoteDSMOld
		} else {
			a.Status = StatusOutdated
			if m == 6 {
				a.Since = "2024-10-01"
			}
		}
		return a, true
	case FritzOS:
		m, ok := major(o.Version)
		switch {
		case o.UpdateAvailable:
			a.Status = StatusUpdate
		case ok && m < 7:
			a.Status, a.Note = StatusOutdated, NoteFritzOld
		case o.Source == SourceFritzBox && ok && m >= 8:
			a.Status = StatusSupported // the box itself says there is nothing newer
		case o.Source == SourceFritzBox && ok:
			// No update offered for FRITZ!OS 7: AVM may no longer maintain the model.
			a.Status, a.Note = StatusAging, NoteFritzEOM
		default:
			return a, false // a version alone does not tell whether an update exists
		}
		return a, true
	}
	return a, false
}

// winBuild holds the end of servicing per edition group. An empty end means
// "supported, end not yet announced". Dates from Microsoft's lifecycle pages.
type winBuild struct {
	build      int
	name       string
	home       string // Home, Pro
	enterprise string // Enterprise, Education
	ltsc       string // LTSC/LTSB and the Server edition sharing the build ("" = none)
}

var winBuilds = []winBuild{
	{10240, "Windows 10 1507", "2017-05-09", "2017-05-09", "2025-10-14"},
	{10586, "Windows 10 1511", "2017-10-10", "2018-04-10", ""},
	{14393, "Windows 10 1607", "2018-04-10", "2019-10-08", "2027-01-12"}, // LTSB 2016 / Server 2016
	{15063, "Windows 10 1703", "2018-10-09", "2019-10-08", ""},
	{16299, "Windows 10 1709", "2019-04-09", "2020-10-13", ""},
	{17134, "Windows 10 1803", "2019-11-12", "2021-05-11", ""},
	{17763, "Windows 10 1809", "2020-11-10", "2021-05-11", "2029-01-09"}, // LTSC 2019 / Server 2019
	{18362, "Windows 10 1903", "2020-12-08", "2020-12-08", ""},
	{18363, "Windows 10 1909", "2021-05-11", "2022-05-10", ""},
	{19041, "Windows 10 2004", "2021-12-14", "2021-12-14", ""},
	{19042, "Windows 10 20H2", "2022-05-10", "2023-05-09", ""},
	{19043, "Windows 10 21H1", "2022-12-13", "2022-12-13", ""},
	{19044, "Windows 10 21H2", "2023-06-13", "2024-06-11", "2027-01-12"}, // LTSC 2021
	{19045, "Windows 10 22H2", "2025-10-14", "2025-10-14", ""},
	{20348, "Windows Server 2022", "2031-10-14", "2031-10-14", "2031-10-14"},
	{22000, "Windows 11 21H2", "2023-10-10", "2024-10-08", ""},
	{22621, "Windows 11 22H2", "2024-10-08", "2025-10-14", ""},
	{22631, "Windows 11 23H2", "2025-11-11", "2026-11-10", ""},
	{26100, "Windows 11 24H2", "2026-10-13", "2027-10-12", "2029-10-09"}, // LTSC 2024 / Server 2025
	{26200, "Windows 11 25H2", "2027-10-12", "2028-10-10", ""},
	{26300, "Windows 11 26H2", "", "", ""},
}

// judgeWindows reads "6.1.7601" or "10.0.26100" (major.minor.build).
func judgeWindows(v string, now time.Time, edition string) (Assessment, bool) {
	a := Assessment{Product: Windows, Version: v}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return a, false
	}
	maj, err1 := strconv.Atoi(parts[0])
	mnr, err2 := strconv.Atoi(parts[1])
	build, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil || build <= 0 {
		return a, false
	}
	a.Version = fmt.Sprintf("Build %d", build)
	if maj < 10 {
		a.Status = StatusOutdated
		switch {
		case maj == 6 && mnr == 1:
			a.Name, a.Since = "Windows 7", "2020-01-14"
		case maj == 6 && mnr == 2:
			a.Name, a.Since = "Windows 8", "2016-01-12"
		case maj == 6 && mnr == 3:
			a.Name, a.Since = "Windows 8.1", "2023-01-10"
		default:
			a.Name = "Windows"
		}
		return a, true
	}
	var best *winBuild
	for i := range winBuilds {
		if winBuilds[i].build == build {
			best = &winBuilds[i]
			break
		}
	}
	if best == nil {
		last := winBuilds[len(winBuilds)-1]
		if build > last.build {
			a.Name, a.Status, a.Note = "Windows 11", StatusSupported, NoteNewerThanKB
			return a, true
		}
		// An unlisted older build (Insider, Server) cannot be named reliably.
		if build < 22000 {
			a.Name = "Windows 10"
		} else {
			a.Name = "Windows 11"
		}
		return a, false
	}
	a.Name = best.name
	end := best.home
	switch edition {
	case EditionEnterprise:
		end, a.Note = best.enterprise, NoteWinEnterprise
	case EditionLTSC:
		if best.ltsc != "" {
			end, a.Note = best.ltsc, NoteWinLTSCSet
		} else {
			end, a.Note = best.enterprise, NoteWinNoLTSC
		}
	}
	switch {
	case end == "":
		a.Status = StatusSupported
	case now.After(mustDate(end)):
		a.Status, a.Since = StatusOutdated, end
	default:
		a.Status, a.Until = StatusSupported, end
	}
	if edition == "" {
		switch {
		case build == 19045:
			a.Note = NoteWinESU
		case a.Status == StatusOutdated && (best.ltsc != "" && now.Before(mustDate(best.ltsc)) || best.enterprise != best.home && now.Before(mustDate(best.enterprise))):
			// Another edition of the same build is still serviced.
			a.Note = NoteWinEditionHint
		}
	}
	return a, true
}

type ubuntuRelease struct {
	version, until, note string
	lts                  bool
}

var ubuntuReleases = []ubuntuRelease{
	{"14.04", "2019-04-30", NoteUbuntuESM, true},
	{"16.04", "2021-04-30", NoteUbuntuESM, true},
	{"18.04", "2023-05-31", NoteUbuntuESM, true},
	{"20.04", "2025-05-31", NoteUbuntuESM, true},
	{"22.04", "2027-06-01", "", true},
	{"24.04", "2029-05-31", "", true},
	{"24.10", "2025-07-10", NoteUbuntuInterm, false},
	{"25.04", "2026-01-15", NoteUbuntuInterm, false},
	{"25.10", "2026-07-09", NoteUbuntuInterm, false},
	{"26.04", "2031-05-31", "", true},
}

func judgeUbuntu(a Assessment, v string, now time.Time) (Assessment, bool) {
	for _, r := range ubuntuReleases {
		if r.version != v {
			continue
		}
		end := mustDate(r.until)
		switch {
		case now.After(end):
			a.Status, a.Since, a.Note = StatusOutdated, r.until, r.note
		case end.Sub(now) < 365*24*time.Hour && r.lts:
			a.Status, a.Until = StatusAging, r.until
		default:
			a.Status, a.Until = StatusSupported, r.until
		}
		return a, true
	}
	return a, false
}

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic("software: bad date " + s)
	}
	return t
}

// major returns the leading number of "18.2", "26", "12".
func major(v string) (int, bool) {
	head, _, _ := strings.Cut(strings.TrimSpace(v), ".")
	n, err := strconv.Atoi(head)
	return n, err == nil && n >= 0 && n < 1000
}

// displayName is the human name of an observation ("iOS 18.2", "Debian 12").
func displayName(o Observation) string {
	label := map[string]string{
		IOS: "iOS", IPadOS: "iPadOS", MacOS: "macOS", TvOS: "tvOS", AudioOS: "audioOS", Android: "Android",
		Debian: "Debian", Ubuntu: "Ubuntu", DSM: "Synology DSM", FritzOS: "FRITZ!OS", ESPHome: "ESPHome",
		Shelly: "Shelly-Firmware", Sonos: "Sonos", Linux: "Linux", OpenSSH: "OpenSSH", Dropbear: "Dropbear SSH",
		BusyBox: "BusyBox", Windows: "Windows", HomeAssistant: "Home Assistant",
	}[o.Product]
	if label == "" {
		label = o.Product
	}
	if o.Product == Windows || o.Version == "" {
		return label
	}
	return label + " " + o.Version
}

// appleMaxOS is the highest major release a model can run.
type appleMaxOS struct {
	product string
	major   int
}

func (m appleMaxOS) label() string {
	return displayName(Observation{Product: m.product, Version: strconv.Itoa(m.major)})
}

// judgeAppleMax decides from the model alone.
func judgeAppleMax(m appleMaxOS) Assessment {
	a := Assessment{Product: m.product, Name: m.label(), MaxOS: m.label()}
	switch {
	case m.major >= appleCurrent:
		// Can run the current release; whether it does is not visible.
		a.Status, a.Name = StatusUnknown, ""
	case m.major >= applePrevious:
		a.Status, a.Note = StatusAging, NoteCannotUpdate
	case m.major == appleLegacy && m.product != MacOS:
		a.Status, a.Note = StatusAging, NoteOlderBranch
	case m.product == MacOS && m.major == macN2:
		a.Status, a.Note = StatusAging, NoteMacN2
	default:
		a.Status, a.Note = StatusOutdated, NoteCannotUpdate
	}
	return a
}

// Highest release per Apple model identifier family and generation.
// Generations not listed here run the current release.
type modelRange struct {
	family   string
	upTo     int // generation numbers up to and including this
	product  string
	maxMajor int
}

var appleModels = []modelRange{
	// iPhone
	{"iPhone", 6, IOS, 12},  // 5s
	{"iPhone", 7, IOS, 12},  // 6, 6 Plus
	{"iPhone", 9, IOS, 15},  // 6s, SE (1st), 7
	{"iPhone", 10, IOS, 16}, // 8, X
	{"iPhone", 11, IOS, 18}, // XS, XR
	// iPad (identifiers are shared across lines; see appleMax)
	{"iPad", 4, IPadOS, 12},
	{"iPad", 5, IPadOS, 15}, // Air 2, mini 4
	// Mac (Intel identifiers; Apple silicon uses "MacNN,N" or newer numbers)
	{"MacBookPro", 14, MacOS, 13},
	{"MacBookPro", 15, MacOS, 15},
	{"MacBookPro", 16, MacOS, 26},
	{"MacBookAir", 7, MacOS, 12},
	{"MacBookAir", 8, MacOS, 14},
	{"MacBookAir", 9, MacOS, 15},
	{"MacBook", 10, MacOS, 13},
	{"iMac", 18, MacOS, 13},
	{"iMac", 19, MacOS, 15},
	{"iMac", 20, MacOS, 26},
	{"Macmini", 7, MacOS, 12},
	{"Macmini", 8, MacOS, 15},
	{"MacPro", 6, MacOS, 12},
	{"MacPro", 7, MacOS, 26},
	{"iMacPro", 1, MacOS, 15},
}

// iPad generations 6 and 7 mix lines with different limits.
var ipadExact = map[string]int{
	"iPad6,3": 16, "iPad6,4": 16, "iPad6,7": 16, "iPad6,8": 16, // iPad Pro (1st gen)
	"iPad6,11": 16, "iPad6,12": 16, // iPad (5th gen)
	"iPad7,1": 17, "iPad7,2": 17, "iPad7,3": 17, "iPad7,4": 17, // iPad Pro (2nd gen)
	"iPad7,5": 17, "iPad7,6": 17, // iPad (6th gen)
	"iPad7,11": 18, "iPad7,12": 18, // iPad (7th gen)
}

// appleMax returns the highest release the model can run, if the model is
// an Apple identifier this table knows to be limited.
func appleMax(model string) (appleMaxOS, bool) {
	model = strings.TrimSpace(model)
	if v, ok := ipadExact[model]; ok {
		return appleMaxOS{IPadOS, v}, true
	}
	family, gen, ok := splitModel(model)
	if !ok {
		return appleMaxOS{}, false
	}
	product := map[string]string{"iPhone": IOS, "iPad": IPadOS, "MacBookPro": MacOS, "MacBookAir": MacOS,
		"MacBook": MacOS, "iMac": MacOS, "iMacPro": MacOS, "Macmini": MacOS, "MacPro": MacOS, "Mac": MacOS}[family]
	if product == "" {
		return appleMaxOS{}, false
	}
	for _, r := range appleModels {
		if r.family == family && gen <= r.upTo {
			return appleMaxOS{r.product, r.maxMajor}, true
		}
	}
	// Newer than every limited generation: runs the current release.
	return appleMaxOS{product, appleCurrent}, true
}

// splitModel splits "iPhone11,8" into ("iPhone", 11).
func splitModel(m string) (string, int, bool) {
	i := strings.IndexAny(m, "0123456789")
	if i <= 0 {
		return "", 0, false
	}
	num, _, ok := strings.Cut(m[i:], ",")
	if !ok {
		return "", 0, false
	}
	n, err := strconv.Atoi(num)
	if err != nil || n <= 0 || n > 999 {
		return "", 0, false
	}
	return m[:i], n, true
}
