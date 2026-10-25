package config

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

// Env is the bootstrap configuration read from environment variables.
type Env struct {
	Port           int
	Listen         string
	DataDir        string
	PUID, PGID     int
	Interfaces     []string
	SetupToken     string
	AllowedHosts   []string
	TrustedProxies []netip.Prefix
	TLSCert        string
	TLSKey         string
	LogLevel       string
	Demo           bool
}

func list(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// LoadEnv reads and validates the environment.
func LoadEnv() (Env, error) {
	e := Env{
		Port:     8840,
		Listen:   "0.0.0.0",
		DataDir:  "/data",
		PUID:     10001,
		PGID:     10001,
		LogLevel: "info",
	}
	get := func(k string) string { return strings.TrimSpace(os.Getenv(k)) }
	num := func(k string, dst *int, lo, hi int) error {
		v := get(k)
		if v == "" {
			return nil
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < lo || n > hi {
			return fmt.Errorf("%s must be a number between %d and %d", k, lo, hi)
		}
		*dst = n
		return nil
	}
	if err := num("NETZWART_PORT", &e.Port, 1, 65535); err != nil {
		return e, err
	}
	if err := num("PUID", &e.PUID, 1, 2147483646); err != nil {
		return e, err
	}
	if err := num("PGID", &e.PGID, 1, 2147483646); err != nil {
		return e, err
	}
	if v := get("NETZWART_LISTEN"); v != "" {
		if _, err := netip.ParseAddr(v); err != nil {
			return e, fmt.Errorf("NETZWART_LISTEN must be an IP address")
		}
		e.Listen = v
	}
	if v := get("NETZWART_DATA"); v != "" {
		e.DataDir = v
	}
	e.Interfaces = list(get("NETZWART_INTERFACES"))
	e.SetupToken = get("NETZWART_SETUP_TOKEN")
	for _, h := range list(get("NETZWART_ALLOWED_HOSTS")) {
		e.AllowedHosts = append(e.AllowedHosts, strings.ToLower(h))
	}
	for _, c := range list(get("NETZWART_TRUSTED_PROXIES")) {
		p, err := netip.ParsePrefix(c)
		if err != nil {
			a, err2 := netip.ParseAddr(c)
			if err2 != nil {
				return e, fmt.Errorf("NETZWART_TRUSTED_PROXIES: %q is not an IP or CIDR", c)
			}
			p = netip.PrefixFrom(a, a.BitLen())
		}
		e.TrustedProxies = append(e.TrustedProxies, p.Masked())
	}
	e.TLSCert, e.TLSKey = get("NETZWART_TLS_CERT"), get("NETZWART_TLS_KEY")
	if (e.TLSCert == "") != (e.TLSKey == "") {
		return e, fmt.Errorf("NETZWART_TLS_CERT and NETZWART_TLS_KEY must be set together")
	}
	if v := strings.ToLower(get("NETZWART_LOG_LEVEL")); v != "" {
		switch v {
		case "debug", "info", "warn", "error":
			e.LogLevel = v
		default:
			return e, fmt.Errorf("NETZWART_LOG_LEVEL must be debug, info, warn or error")
		}
	}
	e.Demo = get("NETZWART_DEMO") == "1" || strings.EqualFold(get("NETZWART_DEMO"), "true")
	return e, nil
}
