// Package config holds the bootstrap configuration (environment variables)
// and the user settings that are edited in the UI and stored in SQLite.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"
)

// Secret is a write-only value: it is stored, but the API never returns it.
// An empty value on update means "keep the current one".
type Secret string

// Mask is what the API returns for secrets that are set.
const Mask = "••••••••"

// MQTT configures the Home Assistant bridge.
type MQTT struct {
	Enabled         bool   `json:"enabled"`
	Host            string `json:"host"`
	Port            int    `json:"port"`
	TLS             bool   `json:"tls"`
	Username        string `json:"username"`
	Password        Secret `json:"password"`
	DiscoveryPrefix string `json:"discoveryPrefix"`
	BaseTopic       string `json:"baseTopic"`
}

// FritzBox configures the optional TR-064 integration.
type FritzBox struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Username string `json:"username"`
	Password Secret `json:"password"`
}

// Settings are the user-editable settings.
type Settings struct {
	NetworkName        string   `json:"networkName"`
	Language           string   `json:"language"` // de | en
	Timezone           string   `json:"timezone"`
	Interfaces         []string `json:"interfaces"`         // empty = interface of the default route
	ScanInterval       int      `json:"scanInterval"`       // seconds
	OfflineAfter       int      `json:"offlineAfter"`       // seconds
	OfflineAfterMobile int      `json:"offlineAfterMobile"` // seconds, phones/tablets/watches
	AllowLargeSubnets  bool     `json:"allowLargeSubnets"`
	ResolveNames       bool     `json:"resolveNames"`
	NotifyNewDevice    bool     `json:"notifyNewDevice"`
	NotifyRandomMAC    bool     `json:"notifyRandomMac"`
	ExpireRandomDays   int      `json:"expireRandomDays"` // 0 = never
	RetentionDays      int      `json:"retentionDays"`
	InternetCheck      bool     `json:"internetCheck"`
	InternetTargets    []string `json:"internetTargets"`
	SecurityScanDays   int      `json:"securityScanDays"` // 0 = only on demand
	DHCPCheck          bool     `json:"dhcpCheck"`        // the security check asks once who hands out addresses
	UpdateCheck        bool     `json:"updateCheck"`      // ask GitHub daily for a new release (off by default)
	MQTT               MQTT     `json:"mqtt"`
	FritzBox           FritzBox `json:"fritzbox"`
}

// Defaults returns the settings of a fresh installation.
func Defaults() Settings {
	return Settings{
		NetworkName:        "",
		Language:           "de",
		Timezone:           "UTC",
		ScanInterval:       90,
		OfflineAfter:       300,
		OfflineAfterMobile: 900,
		ResolveNames:       true,
		NotifyNewDevice:    true,
		NotifyRandomMAC:    true,
		ExpireRandomDays:   14,
		RetentionDays:      90,
		InternetCheck:      true,
		DHCPCheck:          true,
		InternetTargets:    []string{"1.1.1.1:443", "9.9.9.9:443", "dns.google:443"},
		MQTT:               MQTT{Port: 1883, DiscoveryPrefix: "homeassistant", BaseTopic: "netzwart"},
		FritzBox:           FritzBox{Host: "fritz.box"},
	}
}

// OfflineAfterDur returns the offline threshold for a device class.
func (s Settings) OfflineAfterDur(mobile bool) time.Duration {
	if mobile {
		return time.Duration(s.OfflineAfterMobile) * time.Second
	}
	return time.Duration(s.OfflineAfter) * time.Second
}

func validText(s string, max int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= max && !strings.ContainsAny(s, "\x00\r\n")
}

func validHost(h string) bool {
	if h == "" || len(h) > 253 {
		return false
	}
	if _, err := netip.ParseAddr(h); err == nil {
		return true
	}
	for _, r := range h {
		if !(r == '.' || r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

// Validate checks ranges and formats and returns the first problem found.
func (s Settings) Validate() error {
	var errs []error
	check := func(ok bool, field, msg string) {
		if !ok {
			errs = append(errs, fmt.Errorf("%s: %s", field, msg))
		}
	}
	check(validText(s.NetworkName, 64), "networkName", "max. 64 characters")
	check(s.Language == "de" || s.Language == "en", "language", "must be de or en")
	_, tzErr := time.LoadLocation(s.Timezone)
	check(tzErr == nil && validText(s.Timezone, 64), "timezone", "unknown time zone")
	check(len(s.Interfaces) <= 8, "interfaces", "max. 8")
	for _, i := range s.Interfaces {
		check(validText(i, 15) && i != "", "interfaces", "invalid name")
	}
	check(s.ScanInterval >= 30 && s.ScanInterval <= 3600, "scanInterval", "30-3600 seconds")
	check(s.OfflineAfter >= 60 && s.OfflineAfter <= 86400, "offlineAfter", "60-86400 seconds")
	check(s.OfflineAfterMobile >= 60 && s.OfflineAfterMobile <= 86400, "offlineAfterMobile", "60-86400 seconds")
	check(s.OfflineAfter >= 2*s.ScanInterval, "offlineAfter", "must be at least twice the scan interval")
	check(s.ExpireRandomDays >= 0 && s.ExpireRandomDays <= 365, "expireRandomDays", "0-365")
	check(s.RetentionDays >= 7 && s.RetentionDays <= 3650, "retentionDays", "7-3650")
	check(s.SecurityScanDays >= 0 && s.SecurityScanDays <= 90, "securityScanDays", "0-90")
	check(len(s.InternetTargets) >= 1 && len(s.InternetTargets) <= 5, "internetTargets", "1-5 entries")
	for _, t := range s.InternetTargets {
		host, port, ok := strings.Cut(t, ":")
		check(ok && validHost(host) && port != "" && len(port) <= 5, "internetTargets", "use host:port")
	}
	if s.MQTT.Enabled {
		check(validHost(s.MQTT.Host), "mqtt.host", "invalid host")
		check(s.MQTT.Port > 0 && s.MQTT.Port < 65536, "mqtt.port", "1-65535")
		check(validText(s.MQTT.Username, 128) && validText(string(s.MQTT.Password), 256), "mqtt", "invalid credentials")
		check(validTopic(s.MQTT.DiscoveryPrefix) && validTopic(s.MQTT.BaseTopic), "mqtt", "topics may only contain a-z, 0-9, _ - /")
	}
	if s.FritzBox.Enabled {
		check(validHost(s.FritzBox.Host), "fritzbox.host", "invalid host")
		check(validText(s.FritzBox.Username, 128) && validText(string(s.FritzBox.Password), 256), "fritzbox", "invalid credentials")
	}
	return errors.Join(errs...)
}

func validTopic(t string) bool {
	if t == "" || len(t) > 64 || strings.HasPrefix(t, "/") || strings.HasSuffix(t, "/") {
		return false
	}
	for _, r := range t {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '/') {
			return false
		}
	}
	return true
}

// Public returns a copy safe to send to the browser: secrets are masked.
func (s Settings) Public() Settings {
	if s.MQTT.Password != "" {
		s.MQTT.Password = Mask
	}
	if s.FritzBox.Password != "" {
		s.FritzBox.Password = Mask
	}
	s.Interfaces = append([]string{}, s.Interfaces...)
	return s
}

// ErrSecretRequired is returned when a stored secret would be sent to a
// destination it was not entered for.
var ErrSecretRequired = errors.New("secret_required")

// MergeSecrets keeps the current secret where the update leaves it empty or
// masked, but only while the secret's destination is unchanged. Otherwise a
// session without the password could point MQTT or the FRITZ!Box integration
// at its own server and receive the stored credentials.
func (s *Settings) MergeSecrets(current Settings) error {
	keep := func(v *Secret, old Secret, sameDest bool, field string) error {
		if *v != "" && *v != Mask {
			return nil
		}
		if old != "" && !sameDest {
			return fmt.Errorf("%w: %s", ErrSecretRequired, field)
		}
		*v = old
		return nil
	}
	m, cm := s.MQTT, current.MQTT
	if err := keep(&s.MQTT.Password, cm.Password, m.Host == cm.Host && m.Port == cm.Port && m.TLS == cm.TLS && m.Username == cm.Username, "mqtt.password"); err != nil {
		return err
	}
	f, cf := s.FritzBox, current.FritzBox
	return keep(&s.FritzBox.Password, cf.Password, f.Host == cf.Host && f.Username == cf.Username, "fritzbox.password")
}
