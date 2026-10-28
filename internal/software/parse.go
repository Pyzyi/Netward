package software

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Pyzyi/Netward/internal/textutil"
)

// maxEvidence bounds the raw value kept with an observation.
const maxEvidence = 120

func evidence(s string) string { return textutil.Clean(s, maxEvidence) }

var versionRe = regexp.MustCompile(`^[0-9]{1,4}(\.[0-9]{1,6}){0,3}$`)

// FromTXT reads a whitelisted TXT record (proto.VersionTXT) of one service.
func FromTXT(service string, kv map[string]string) []Observation {
	get := func(k string) string { return strings.TrimSpace(kv[k]) }
	switch service {
	case "_airplay._tcp":
		v := get("osvers")
		if !versionRe.MatchString(v) {
			return nil
		}
		p := appleProductForModel(get("model"))
		if p == "" {
			return nil
		}
		return []Observation{{Source: SourceAirPlay, Product: p, Version: v, Evidence: evidence("osvers=" + v + " model=" + get("model"))}}
	case "_device-info._tcp":
		v := get("osxvers")
		n, err := strconv.Atoi(v)
		if err != nil || n < 10 || n > 99 {
			return nil
		}
		return []Observation{{Source: SourceDeviceInfo, Product: MacOS, Version: strconv.Itoa(darwinToMacOS(n)), Evidence: evidence("osxvers=" + v)}}
	case "_http._tcp":
		if !strings.EqualFold(get("vendor"), "Synology") {
			return nil
		}
		maj, mnr, build := get("version_major"), get("version_minor"), get("version_build")
		if _, err := strconv.Atoi(maj); err != nil {
			return nil
		}
		v := maj
		if _, err := strconv.Atoi(mnr); err == nil {
			v += "." + mnr
		}
		if _, err := strconv.Atoi(build); err == nil {
			v += " (Build " + build + ")"
		}
		return []Observation{{Source: SourceSynology, Product: DSM, Version: v, Evidence: evidence(fmt.Sprintf("version_major=%s version_minor=%s version_build=%s", maj, mnr, build))}}
	case "_esphomelib._tcp":
		v := get("version")
		if !versionRe.MatchString(v) {
			return nil
		}
		return []Observation{{Source: SourceESPHome, Product: ESPHome, Version: v, Evidence: evidence("version=" + v)}}
	case "_shelly._tcp":
		v := strings.TrimPrefix(get("ver"), "v")
		if !versionRe.MatchString(v) {
			return nil
		}
		return []Observation{{Source: SourceShelly, Product: Shelly, Version: v, Evidence: evidence("ver=" + get("ver"))}}
	case "_home-assistant._tcp":
		v := get("version")
		if !versionRe.MatchString(v) {
			return nil
		}
		return []Observation{{Source: SourceHomeAssistant, Product: HomeAssistant, Version: v, Evidence: evidence("version=" + v)}}
	}
	return nil
}

// darwinToMacOS maps the Darwin major version to the macOS major version
// (Darwin 20 = macOS 11 … Darwin 24 = macOS 15, Darwin 25 = macOS 26).
func darwinToMacOS(d int) int {
	switch {
	case d >= 25:
		return d + 1
	case d >= 20:
		return d - 9
	default:
		return 10 // 10.x: everything before Big Sur
	}
}

// appleProductForModel tells which system an AirPlay osvers= refers to.
func appleProductForModel(model string) string {
	switch {
	case strings.HasPrefix(model, "AppleTV"):
		return TvOS
	case strings.HasPrefix(model, "AudioAccessory"):
		return AudioOS
	case strings.HasPrefix(model, "Mac"), strings.HasPrefix(model, "iMac"):
		return MacOS
	case strings.HasPrefix(model, "iPhone"):
		return IOS
	case strings.HasPrefix(model, "iPad"):
		return IPadOS
	}
	return ""
}

var (
	// "FRITZ!Box 7590 UPnP/1.0 AVM FRITZ!Box 7590 154.07.57": hardware code, major, minor.
	fritzRe = regexp.MustCompile(`AVM FRITZ!\S+(?: [^/]*?)? (\d{2,3})\.(\d{2})\.(\d{2})(?:-\d+)?\s*$`)
	// "Linux UPnP/1.0 Sonos/80.1-55014 (ZPS27)"
	sonosRe = regexp.MustCompile(`Sonos/(\d{1,3}\.\d{1,3})(?:-\d+)?`)
)

// FromSSDPServer reads the SERVER header of an SSDP response.
func FromSSDPServer(server string) []Observation {
	server = strings.TrimSpace(server)
	if len(server) > 256 {
		return nil
	}
	if m := fritzRe.FindStringSubmatch(server); m != nil {
		maj, _ := strconv.Atoi(m[2])
		v := fmt.Sprintf("%d.%s", maj, m[3])
		return []Observation{{Source: SourceSSDP, Product: FritzOS, Version: v, Evidence: evidence(server)}}
	}
	if m := sonosRe.FindStringSubmatch(server); m != nil {
		return []Observation{{Source: SourceSSDP, Product: Sonos, Version: m[1], Evidence: evidence(server)}}
	}
	return nil
}

var (
	androidRe = regexp.MustCompile(`^android-dhcp-(\d{1,2})(?:\.\d+)*$`)
	dhcpcdRe  = regexp.MustCompile(`^dhcpcd-[0-9.]+:Linux-(\d+\.\d+(?:\.\d+)?)`)
	udhcpRe   = regexp.MustCompile(`^udhcp[c]? (\d+\.\d+(?:\.\d+)?)$`)
)

// FromDHCPVendor reads DHCP option 60 (vendor class identifier).
func FromDHCPVendor(vc string) []Observation {
	vc = strings.TrimSpace(vc)
	if len(vc) > 255 {
		return nil
	}
	if m := androidRe.FindStringSubmatch(vc); m != nil {
		return []Observation{{Source: SourceDHCP, Product: Android, Version: m[1], Evidence: evidence(vc)}}
	}
	if m := dhcpcdRe.FindStringSubmatch(vc); m != nil {
		return []Observation{{Source: SourceDHCP, Product: Linux, Version: m[1], Evidence: evidence(vc)}}
	}
	if m := udhcpRe.FindStringSubmatch(vc); m != nil {
		return []Observation{{Source: SourceDHCP, Product: BusyBox, Version: m[1], Evidence: evidence(vc)}}
	}
	return nil
}

var (
	sshRe     = regexp.MustCompile(`^SSH-(?:2\.0|1\.99)-([A-Za-z0-9._+-]{1,64})(?: (.{1,128}))?$`)
	opensshRe = regexp.MustCompile(`^OpenSSH_(\d+\.\d+)(p\d+)?`)
	debRe     = regexp.MustCompile(`\+deb(\d{1,2})u\d+`)
)

// OpenSSH version shipped by each Debian release (when the comment lacks +debN).
var debianOpenSSH = map[string]string{"6.7": "8", "7.4": "9", "7.9": "10", "8.4": "11", "9.2": "12", "10.0": "13"}

// OpenSSH version shipped by each Ubuntu release.
var ubuntuOpenSSH = map[string]string{"6.6": "14.04", "7.2": "16.04", "7.6": "18.04", "8.2": "20.04", "8.9": "22.04",
	"9.6": "24.04", "9.7": "24.10", "9.9": "25.04", "10.0": "25.10", "10.2": "26.04"}

// FromSSHBanner reads an SSH identification line ("SSH-2.0-OpenSSH_9.2p1 Debian-2+deb12u7").
func FromSSHBanner(line string) []Observation {
	line = strings.TrimRight(line, "\r\n")
	m := sshRe.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	soft, comment := m[1], m[2]
	ev := evidence(line)
	if strings.HasPrefix(strings.ToLower(soft), "dropbear") {
		v := strings.TrimPrefix(strings.TrimPrefix(soft, "dropbear_"), "dropbear")
		return []Observation{{Source: SourceSSH, Product: Dropbear, Version: v, Evidence: ev}}
	}
	om := opensshRe.FindStringSubmatch(soft)
	if om == nil {
		return nil
	}
	ossh := om[1]
	lc := strings.ToLower(comment)
	switch {
	case strings.HasPrefix(lc, "debian-") || strings.HasPrefix(lc, "raspbian-"):
		if dm := debRe.FindStringSubmatch(comment); dm != nil {
			return []Observation{{Source: SourceSSH, Product: Debian, Version: dm[1], Evidence: ev}}
		}
		// Unknown pairing (testing, backports): the distribution, not the release.
		return []Observation{{Source: SourceSSH, Product: Debian, Version: debianOpenSSH[ossh], Evidence: ev}}
	case strings.HasPrefix(lc, "ubuntu-"):
		return []Observation{{Source: SourceSSH, Product: Ubuntu, Version: ubuntuOpenSSH[ossh], Evidence: ev}}
	}
	return []Observation{{Source: SourceSSH, Product: OpenSSH, Version: ossh + om[2], Evidence: ev}}
}

// FromNTLMVersion turns the version field of an NTLM CHALLENGE into an
// observation. Samba fills in a fixed placeholder without a build number;
// such answers are not Windows and are ignored.
//
// Only version pairs Windows actually reports are accepted (6.1, 6.2, 6.3,
// 10.0) with a plausible build number; Samba, NAS firmware and macOS answer
// with placeholders that must never turn into "Windows 7".
func FromNTLMVersion(major, minor uint8, build uint16) []Observation {
	switch {
	case major == 6 && minor == 1 && (build == 7600 || build == 7601),
		major == 6 && minor == 2 && build == 9200,
		major == 6 && minor == 3 && build == 9600,
		major == 10 && minor == 0 && build >= 10240:
	default:
		return nil
	}
	v := fmt.Sprintf("%d.%d.%d", major, minor, build)
	return []Observation{{Source: SourceSMB, Product: Windows, Version: v, Evidence: "NTLM " + v}}
}

// FromFritzBox reads what TR-064 says about the router's own firmware:
// version "154.08.25" (hardware code, major, minor) and whether an update waits.
func FromFritzBox(version string, upgradeAvailable bool, newVersion string) []Observation {
	version = strings.TrimSpace(version)
	if i := strings.IndexByte(version, '-'); i > 0 { // lab builds: "154.07.59-112345"
		version = version[:i]
	}
	parts := strings.Split(version, ".")
	if len(parts) < 3 {
		return nil
	}
	maj, err1 := strconv.Atoi(parts[len(parts)-2])
	mnr, err2 := strconv.Atoi(parts[len(parts)-1])
	if err1 != nil || err2 != nil || maj > 99 || mnr > 99 {
		return nil
	}
	ev := "SoftwareVersion=" + version
	if upgradeAvailable && newVersion != "" {
		ev += " Update=" + newVersion
	}
	return []Observation{{Source: SourceFritzBox, Product: FritzOS, Version: fmt.Sprintf("%d.%02d", maj, mnr),
		Evidence: evidence(ev), UpdateAvailable: upgradeAvailable}}
}
