package config

import (
	"strings"
	"testing"
)

func TestDefaultsAreValid(t *testing.T) {
	if err := Defaults().Validate(); err != nil {
		t.Fatalf("defaults invalid: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]func(*Settings){
		"interval too short":    func(s *Settings) { s.ScanInterval = 10 },
		"offline below 2x scan": func(s *Settings) { s.OfflineAfter = 100 },
		"bad language":          func(s *Settings) { s.Language = "fr" },
		"bad timezone":          func(s *Settings) { s.Timezone = "Mars/Olympus" },
		"newline in name":       func(s *Settings) { s.NetworkName = "a\nb" },
		"bad target":            func(s *Settings) { s.InternetTargets = []string{"no-port"} },
		"mqtt topic injection":  func(s *Settings) { s.MQTT.Enabled = true; s.MQTT.Host = "1.2.3.4"; s.MQTT.BaseTopic = "a/+/#" },
		"mqtt bad host":         func(s *Settings) { s.MQTT.Enabled = true; s.MQTT.Host = "evil host" },
		"retention too short":   func(s *Settings) { s.RetentionDays = 1 },
	}
	for name, mutate := range cases {
		s := Defaults()
		mutate(&s)
		if err := s.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSecretsMaskedAndMerged(t *testing.T) {
	cur := Defaults()
	cur.MQTT.Password = "geheim"
	pub := cur.Public()
	if pub.MQTT.Password != Mask {
		t.Fatalf("password not masked: %q", pub.MQTT.Password)
	}
	upd := pub
	upd.MergeSecrets(cur)
	if upd.MQTT.Password != "geheim" {
		t.Fatal("masked value replaced the stored secret")
	}
	upd.MQTT.Password = "neu"
	upd.MergeSecrets(cur)
	if upd.MQTT.Password != "neu" {
		t.Fatal("new secret not taken")
	}
}

func TestLoadEnv(t *testing.T) {
	t.Setenv("NETWARD_PORT", "9000")
	t.Setenv("NETWARD_ALLOWED_HOSTS", "Netward.Example.com, *.lan.example")
	t.Setenv("NETWARD_TRUSTED_PROXIES", "10.0.0.0/8,192.168.1.2")
	t.Setenv("PUID", "1000")
	e, err := LoadEnv()
	if err != nil {
		t.Fatal(err)
	}
	if e.Port != 9000 || e.PUID != 1000 || len(e.AllowedHosts) != 2 || e.AllowedHosts[0] != "netward.example.com" || len(e.TrustedProxies) != 2 {
		t.Fatalf("unexpected env %+v", e)
	}
	for k, v := range map[string]string{"NETWARD_PORT": "70000", "NETWARD_LISTEN": "not-an-ip", "NETWARD_TRUSTED_PROXIES": "foo", "NETWARD_LOG_LEVEL": "loud"} {
		t.Run(k, func(t *testing.T) {
			t.Setenv(k, v)
			if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "NETWARD") {
				t.Fatalf("%s=%s accepted (%v)", k, v, err)
			}
		})
	}
	t.Setenv("NETWARD_TLS_CERT", "/x.pem")
	if _, err := LoadEnv(); err == nil {
		t.Fatal("certificate without key accepted")
	}
}

func TestLoadEnvOldNames(t *testing.T) {
	t.Setenv("NETZWART_PORT", "9000") // installation from before the rename
	t.Setenv("NETZWART_DEMO", "en")
	t.Setenv("NETWARD_LOG_LEVEL", "debug")
	t.Setenv("NETZWART_LOG_LEVEL", "error") // the new name wins
	e, err := LoadEnv()
	if err != nil {
		t.Fatal(err)
	}
	if e.Port != 9000 || !e.Demo || e.DemoLang != "en" || e.LogLevel != "debug" {
		t.Fatalf("env = %+v", e)
	}
}
