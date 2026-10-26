package software

import (
	"testing"
	"time"
)

var now = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

func one(t *testing.T, obs []Observation) Observation {
	t.Helper()
	if len(obs) != 1 {
		t.Fatalf("want one observation, got %+v", obs)
	}
	return obs[0]
}

func TestParsers(t *testing.T) {
	cases := []struct {
		name          string
		obs           []Observation
		product, vers string
	}{
		{"airplay appletv", FromTXT("_airplay._tcp", map[string]string{"osvers": "26.1", "model": "AppleTV14,1"}), TvOS, "26.1"},
		{"airplay homepod", FromTXT("_airplay._tcp", map[string]string{"osvers": "18.4", "model": "AudioAccessory5,1"}), AudioOS, "18.4"},
		{"airplay mac", FromTXT("_airplay._tcp", map[string]string{"osvers": "15.6", "model": "Mac14,2"}), MacOS, "15.6"},
		{"device-info darwin 23", FromTXT("_device-info._tcp", map[string]string{"osxvers": "23", "model": "MacBookAir10,1"}), MacOS, "14"},
		{"device-info darwin 25", FromTXT("_device-info._tcp", map[string]string{"osxvers": "25"}), MacOS, "26"},
		{"synology", FromTXT("_http._tcp", map[string]string{"vendor": "Synology", "version_major": "7", "version_minor": "2", "version_build": "72806"}), DSM, "7.2 (Build 72806)"},
		{"esphome", FromTXT("_esphomelib._tcp", map[string]string{"version": "2024.6.1"}), ESPHome, "2024.6.1"},
		{"shelly", FromTXT("_shelly._tcp", map[string]string{"ver": "1.3.2", "gen": "2"}), Shelly, "1.3.2"},
		{"fritz ssdp", FromSSDPServer("FRITZ!Box 7590 UPnP/1.0 AVM FRITZ!Box 7590 154.07.57"), FritzOS, "7.57"},
		{"fritz ssdp 8", FromSSDPServer("FRITZ!Box 7690 UPnP/1.0 AVM FRITZ!Box 7690 279.08.05"), FritzOS, "8.05"},
		{"sonos", FromSSDPServer("Linux UPnP/1.0 Sonos/80.1-55014 (ZPS27)"), Sonos, "80.1"},
		{"android", FromDHCPVendor("android-dhcp-14"), Android, "14"},
		{"dhcpcd", FromDHCPVendor("dhcpcd-9.4.1:Linux-6.1.21-v8+:aarch64:Raspberry Pi 4 Model B Rev 1.4"), Linux, "6.1.21"},
		{"udhcp", FromDHCPVendor("udhcp 1.36.1"), BusyBox, "1.36.1"},
		{"ssh debian deb12", FromSSHBanner("SSH-2.0-OpenSSH_9.2p1 Debian-2+deb12u7\r\n"), Debian, "12"},
		{"ssh raspbian deb11", FromSSHBanner("SSH-2.0-OpenSSH_8.4p1 Raspbian-5+deb11u3"), Debian, "11"},
		{"ssh debian by openssh", FromSSHBanner("SSH-2.0-OpenSSH_7.4p1 Debian-10"), Debian, "9"},
		{"ssh ubuntu", FromSSHBanner("SSH-2.0-OpenSSH_8.9p1 Ubuntu-3ubuntu0.10"), Ubuntu, "22.04"},
		{"ssh ubuntu 20.04", FromSSHBanner("SSH-2.0-OpenSSH_8.2p1 Ubuntu-4ubuntu0.11"), Ubuntu, "20.04"},
		{"ssh plain", FromSSHBanner("SSH-2.0-OpenSSH_9.8"), OpenSSH, "9.8"},
		{"ssh dropbear", FromSSHBanner("SSH-2.0-dropbear_2022.83"), Dropbear, "2022.83"},
		{"ntlm win10", FromNTLMVersion(10, 0, 19045), Windows, "10.0.19045"},
		{"fritz tr064", FromFritzBox("154.07.57", true, "154.08.25"), FritzOS, "7.57"},
	}
	for _, c := range cases {
		o := one(t, c.obs)
		if o.Product != c.product || o.Version != c.vers {
			t.Errorf("%s: got %s %q, want %s %q", c.name, o.Product, o.Version, c.product, c.vers)
		}
		if o.Evidence == "" {
			t.Errorf("%s: no evidence", c.name)
		}
	}
	for name, obs := range map[string][]Observation{
		"airplay without model": FromTXT("_airplay._tcp", map[string]string{"osvers": "26.1"}),
		"not synology":          FromTXT("_http._tcp", map[string]string{"vendor": "QNAP", "version_major": "5"}),
		"bad osvers":            FromTXT("_airplay._tcp", map[string]string{"osvers": "26.1; drop", "model": "AppleTV14,1"}),
		"samba ntlm":            FromNTLMVersion(6, 1, 0),
		"odd ntlm":              FromNTLMVersion(6, 1, 1234),
		"tiny build":            FromNTLMVersion(10, 0, 5),
		"windows msft":          FromDHCPVendor("MSFT 5.0"),
		"no ssh":                FromSSHBanner("HTTP/1.1 400 Bad Request"),
		"unknown ssdp":          FromSSDPServer("Linux/3.10 UPnP/1.0 IpBridge/1.26.0"),
	} {
		if len(obs) != 0 {
			t.Errorf("%s: want nothing, got %+v", name, obs)
		}
	}
}

func TestAssess(t *testing.T) {
	obs := func(src, product, version string) []Observation {
		return []Observation{{Source: src, Product: product, Version: version, SeenAt: 1}}
	}
	cases := []struct {
		name   string
		in     Input
		status string
		label  string
		note   string
	}{
		{"win10 22h2", Input{Observations: obs(SourceSMB, Windows, "10.0.19045")}, StatusOutdated, "Windows 10 22H2", NoteWinESU},
		{"win11 24h2", Input{Observations: obs(SourceSMB, Windows, "10.0.26100")}, StatusOutdated, "Windows 11 24H2", NoteWinServer},
		{"win11 25h2", Input{Observations: obs(SourceSMB, Windows, "10.0.26200")}, StatusSupported, "Windows 11 25H2", ""},
		{"win11 newer than kb", Input{Observations: obs(SourceSMB, Windows, "10.0.26500")}, StatusSupported, "Windows 11", NoteNewerThanKB},
		{"win 21h2 home", Input{Observations: obs(SourceSMB, Windows, "10.0.19044")}, StatusOutdated, "Windows 10 21H2", NoteWinLTSC},
		{"win ltsc 2021", Input{Edition: EditionExtended, Observations: obs(SourceSMB, Windows, "10.0.19044")}, StatusSupported, "Windows 10 21H2", NoteWinExtended},
		{"win 24h2 enterprise", Input{Edition: EditionExtended, Observations: obs(SourceSMB, Windows, "10.0.26100")}, StatusSupported, "Windows 11 24H2", NoteWinExtended},
		{"edition ignored for 22h2", Input{Edition: EditionExtended, Observations: obs(SourceSMB, Windows, "10.0.19045")}, StatusOutdated, "Windows 10 22H2", NoteWinESU},
		{"win7", Input{Observations: obs(SourceSMB, Windows, "6.1.7601")}, StatusOutdated, "Windows 7", ""},
		{"ios 27", Input{Observations: obs(SourceAirPlay, IOS, "27.0.1")}, StatusSupported, "iOS 27.0.1", ""},
		{"tvos 18", Input{Observations: obs(SourceAirPlay, TvOS, "18.6")}, StatusOutdated, "tvOS 18.6", ""},
		{"tvos 26", Input{Observations: obs(SourceAirPlay, TvOS, "26.4")}, StatusUpdate, "tvOS 26.4", NoteAppleTVUpdate},
		{"tvos 27", Input{Observations: obs(SourceAirPlay, TvOS, "27.0")}, StatusSupported, "tvOS 27.0", ""},
		{"ios 18", Input{Observations: obs(SourceManual, IOS, "18")}, StatusAging, "iOS 18", NoteOlderBranch},
		{"tvos 16", Input{Observations: obs(SourceAirPlay, TvOS, "16.1")}, StatusOutdated, "tvOS 16.1", ""},
		{"macos 15", Input{Observations: obs(SourceDeviceInfo, MacOS, "15")}, StatusAging, "macOS 15", NoteMacN2},
		{"macos 14", Input{Observations: obs(SourceDeviceInfo, MacOS, "14")}, StatusOutdated, "macOS 14", ""},
		{"android 13", Input{Observations: obs(SourceDHCP, Android, "13")}, StatusOutdated, "Android 13", NoteAndroidOEM},
		{"android 16", Input{Observations: obs(SourceDHCP, Android, "16")}, StatusSupported, "Android 16", NoteAndroidOEM},
		{"debian 11", Input{Observations: obs(SourceSSH, Debian, "11")}, StatusOutdated, "Debian 11", ""},
		{"debian 12", Input{Observations: obs(SourceSSH, Debian, "12")}, StatusSupported, "Debian 12", NoteDebianLTS},
		{"ubuntu 20.04", Input{Observations: obs(SourceSSH, Ubuntu, "20.04")}, StatusOutdated, "Ubuntu 20.04", NoteUbuntuESM},
		{"ubuntu 22.04", Input{Observations: obs(SourceSSH, Ubuntu, "22.04")}, StatusAging, "Ubuntu 22.04", ""},
		{"dsm 6", Input{Observations: obs(SourceSynology, DSM, "6.2 (Build 25556)")}, StatusOutdated, "Synology DSM 6.2 (Build 25556)", ""},
		{"fritz update", Input{Observations: FromFritzBox("154.07.57", true, "154.08.25")}, StatusUpdate, "FRITZ!OS 7.57", ""},
		{"fritz current", Input{Observations: FromFritzBox("154.08.25", false, "")}, StatusSupported, "FRITZ!OS 8.25", ""},
		{"fritz 7 no update", Input{Observations: FromFritzBox("113.07.60", false, "")}, StatusAging, "FRITZ!OS 7.60", NoteFritzEOM},
		{"dsm 7.1", Input{Observations: obs(SourceSynology, DSM, "7.1 (Build 42962)")}, StatusAging, "Synology DSM 7.1 (Build 42962)", NoteDSMOld},
		{"dsm 7.2", Input{Observations: obs(SourceSynology, DSM, "7.2 (Build 72806)")}, StatusSupported, "Synology DSM 7.2 (Build 72806)", ""},
		{"debian unknown release", Input{Observations: FromSSHBanner("SSH-2.0-OpenSSH_10.2p1 Debian-1")}, StatusUnknown, "Debian", ""},
		{"fritz ssdp only", Input{Observations: FromSSDPServer("FRITZ!Box 7590 UPnP/1.0 AVM FRITZ!Box 7590 154.07.57")}, StatusUnknown, "FRITZ!OS 7.57", ""},
		{"fritz 6", Input{Observations: FromSSDPServer("FRITZ!Box 7490 UPnP/1.0 AVM FRITZ!Box 7490 113.06.93")}, StatusOutdated, "FRITZ!OS 6.93", NoteFritzOld},
		{"iphone x by model", Input{Model: "iPhone10,3"}, StatusOutdated, "iOS 16", NoteCannotUpdate},
		{"iphone xs by model", Input{Model: "iPhone11,2"}, StatusAging, "iOS 18", NoteOlderBranch},
		{"iphone 11 by model", Input{Model: "iPhone12,1"}, StatusUnknown, "", ""},
		{"ipad 7th gen", Input{Model: "iPad7,11"}, StatusAging, "iPadOS 18", NoteOlderBranch},
		{"ipad 6th gen", Input{Model: "iPad7,5"}, StatusOutdated, "iPadOS 17", NoteCannotUpdate},
		{"intel mac 2019", Input{Model: "MacBookPro16,1"}, StatusAging, "macOS 26", NoteCannotUpdate},
		{"intel mac 2018", Input{Model: "MacBookPro15,1"}, StatusAging, "macOS 15", NoteMacN2},
		{"intel mac 2017", Input{Model: "MacBookPro14,1"}, StatusOutdated, "macOS 13", NoteCannotUpdate},
		{"apple silicon", Input{Model: "Mac14,2"}, StatusUnknown, "", ""},
		{"esphome only", Input{Observations: obs(SourceESPHome, ESPHome, "2024.6.1")}, StatusUnknown, "ESPHome 2024.6.1", ""},
		{"nothing", Input{}, StatusUnknown, "", ""},
	}
	for _, c := range cases {
		c.in.Now = now
		for i := range c.in.Observations {
			c.in.Observations[i].SeenAt = now.UnixMilli()
		}
		a := Assess(c.in)
		if a.Status != c.status || a.Name != c.label || a.Note != c.note {
			t.Errorf("%s: got %s %q note %q, want %s %q note %q", c.name, a.Status, a.Name, a.Note, c.status, c.label, c.note)
		}
		if a.DataAsOf != DataAsOf || a.Observations == nil {
			t.Errorf("%s: missing data stamp or observations", c.name)
		}
	}
}

func TestAssessPrefersPreciseSource(t *testing.T) {
	a := Assess(Input{Now: now, Model: "MacBookPro16,1", Observations: []Observation{
		{Source: SourceDHCP, Product: Linux, Version: "6.1", SeenAt: now.UnixMilli()},
		{Source: SourceDeviceInfo, Product: MacOS, Version: "26", SeenAt: now.UnixMilli() - 1000},
	}})
	if a.Source != SourceDeviceInfo || a.Status != StatusSupported || a.MaxOS != "macOS 26" {
		t.Fatalf("got %+v", a)
	}
	if a.Observations[0].Source != SourceDeviceInfo {
		t.Fatalf("observations not sorted: %+v", a.Observations)
	}
}

func TestStaleAndConflict(t *testing.T) {
	old := now.Add(-100 * 24 * time.Hour).UnixMilli()
	a := Assess(Input{Now: now, Observations: []Observation{{Source: SourceSMB, Product: Windows, Version: "10.0.19045", SeenAt: old}}})
	if a.Status != StatusUnknown || a.Note != NoteStale || a.LastSeen != old || a.Name != "Windows 10 22H2" {
		t.Fatalf("stale observation still decides: %+v", a)
	}
	// A fresh passive source wins over a stale active one.
	a = Assess(Input{Now: now, Observations: []Observation{
		{Source: SourceSMB, Product: Windows, Version: "10.0.19045", SeenAt: old},
		{Source: SourceDHCP, Product: Android, Version: "16", SeenAt: now.UnixMilli()},
	}})
	if a.Product != Android || a.Conflict {
		t.Fatalf("fresh source should decide without conflict: %+v", a)
	}
	// Two fresh sources naming different systems are flagged.
	a = Assess(Input{Now: now, Observations: []Observation{
		{Source: SourceSMB, Product: Windows, Version: "10.0.26200", SeenAt: now.UnixMilli()},
		{Source: SourceAirPlay, Product: MacOS, Version: "26.1", SeenAt: now.UnixMilli()},
	}})
	if a.Product != Windows || !a.Conflict {
		t.Fatalf("conflict not flagged: %+v", a)
	}
}

func TestSupportedUntilExpires(t *testing.T) {
	a := Assess(Input{Now: time.Date(2027, 10, 13, 0, 0, 0, 0, time.UTC), Observations: []Observation{{Source: SourceSMB, Product: Windows, Version: "10.0.26200"}}})
	if a.Status != StatusOutdated || a.Since != "2027-10-12" {
		t.Fatalf("25H2 after its end date: %+v", a)
	}
}

func FuzzSSHBanner(f *testing.F) {
	for _, s := range []string{"SSH-2.0-OpenSSH_9.2p1 Debian-2+deb12u7", "SSH-2.0-dropbear_2022.83", "SSH-1.99-x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, o := range FromSSHBanner(s) {
			if len(o.Evidence) > maxEvidence*4 || len(o.Version) > 80 {
				t.Fatalf("unbounded output %+v", o)
			}
		}
	})
}

func FuzzSSDPServer(f *testing.F) {
	f.Add("FRITZ!Box 7590 UPnP/1.0 AVM FRITZ!Box 7590 154.07.57")
	f.Add("Linux UPnP/1.0 Sonos/80.1-55014 (ZPS27)")
	f.Fuzz(func(t *testing.T, s string) {
		for _, o := range FromSSDPServer(s) {
			_ = Assess(Input{Now: now, Observations: []Observation{o}})
		}
	})
}
