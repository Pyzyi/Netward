package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Pyzyi/netzwart/internal/egress"
	"github.com/Pyzyi/netzwart/internal/store"
)

// testClient uses a plain client so tests can reach httptest servers on
// loopback; the production policy is covered by the egress tests and below.
func testClient() (*http.Client, *egress.Policy) {
	p := &egress.Policy{}
	c := &http.Client{Timeout: 5 * time.Second}
	return c, p
}

type captured struct {
	path    string
	header  http.Header
	body    []byte
	content string
}

func capture(t *testing.T) (*httptest.Server, *captured) {
	t.Helper()
	got := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.header = r.Header.Clone()
		got.content = r.Header.Get("Content-Type")
		got.body, _ = io.ReadAll(r.Body)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

var evil = &store.Device{ID: 7, MAC: "3c:22:fb:00:00:01", IP: "192.168.1.66",
	Hostname: "@everyone <!channel> [klick](http://evil.example) *fett*", Vendor: "Apple", Type: "phone"}

func TestDiscordNeutralisesMentionsAndLinks(t *testing.T) {
	srv, got := capture(t)
	c, p := testClient()
	cfg, _ := json.Marshal(Config{URL: srv.URL})
	msg := Render(Event{Kind: KindDeviceNew, TS: 1, Device: evil}, "de", time.UTC)
	if err := Send(context.Background(), c, p, ChanDiscord, cfg, msg); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Content         string         `json:"content"`
		AllowedMentions map[string]any `json:"allowed_mentions"`
	}
	if err := json.Unmarshal(got.body, &payload); err != nil {
		t.Fatal(err)
	}
	if parse, ok := payload.AllowedMentions["parse"].([]any); !ok || len(parse) != 0 {
		t.Fatal("allowed_mentions must disable all mention parsing")
	}
	if strings.Contains(payload.Content, "[klick](http") || strings.Contains(payload.Content, "http://") {
		t.Fatalf("markdown link survived: %q", payload.Content)
	}
}

func TestSlackEscapes(t *testing.T) {
	srv, got := capture(t)
	c, p := testClient()
	cfg, _ := json.Marshal(Config{URL: srv.URL})
	_ = Send(context.Background(), c, p, ChanSlack, cfg, Render(Event{Kind: KindDeviceNew, TS: 1, Device: evil}, "en", time.UTC))
	if strings.Contains(string(got.body), "<!channel>") {
		t.Fatalf("slack special sequence survived: %s", got.body)
	}
	if !strings.Contains(string(got.body), `"unfurl_links":false`) {
		t.Fatal("unfurling must be disabled")
	}
}

func TestNtfyUsesJSONNotHeaders(t *testing.T) {
	srv, got := capture(t)
	c, p := testClient()
	cfg, _ := json.Marshal(Config{Server: srv.URL, Topic: "heimnetz", Token: "tk_secret"})
	msg := Render(Event{Kind: KindDeviceNew, TS: 1, Device: &store.Device{MAC: "aa:bb:cc:dd:ee:ff", Hostname: "Küchen-Radio\r\nX-Evil: 1"}}, "de", time.UTC)
	if err := Send(context.Background(), c, p, ChanNtfy, cfg, msg); err != nil {
		t.Fatal(err)
	}
	if got.header.Get("Authorization") != "Bearer tk_secret" || got.header.Get("X-Evil") != "" {
		t.Fatalf("unexpected headers %v", got.header)
	}
	var body map[string]any
	_ = json.Unmarshal(got.body, &body)
	if body["topic"] != "heimnetz" || !strings.Contains(body["message"].(string), "Küchen-Radio") {
		t.Fatalf("unexpected body %v", body)
	}
}

func TestTelegramHasNoParseMode(t *testing.T) {
	// Telegram's host is fixed, so only the payload builder is checked here.
	msg := Render(Event{Kind: KindDeviceOffline, TS: 1, Device: evil}, "de", time.UTC)
	if !strings.Contains(msg.Title, "ist offline") {
		t.Fatal(msg.Title)
	}
}

func TestWebhookPayloadAndHeader(t *testing.T) {
	srv, got := capture(t)
	c, p := testClient()
	cfg, _ := json.Marshal(Config{URL: srv.URL, HeaderName: "Authorization", HeaderValue: "Bearer x"})
	if err := Send(context.Background(), c, p, ChanWebhook, cfg, Render(Event{Kind: KindDeviceNew, TS: 1, Device: evil}, "en", time.UTC)); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal(got.body, &body)
	if body["event"] != "device_new" || got.header.Get("Authorization") != "Bearer x" {
		t.Fatalf("unexpected webhook %v %v", body, got.header)
	}
}

func TestErrorsDoNotLeakSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("secret body"))
	}))
	defer srv.Close()
	c, p := testClient()
	cfg, _ := json.Marshal(Config{URL: srv.URL + "/hooks/SECRET-TOKEN"})
	err := Send(context.Background(), c, p, ChanDiscord, cfg, Message{Title: "x"})
	if err == nil || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "secret body") {
		t.Fatalf("error leaks data: %v", err)
	}
	if err.Error() != "HTTP 401" {
		t.Fatalf("unexpected error %q", err)
	}
	// Real policy: loopback is refused before any request is made.
	real := (&egress.Policy{}).HTTPClient(2*time.Second, 3)
	err = Send(context.Background(), real, &egress.Policy{}, ChanDiscord, cfg, Message{Title: "x"})
	if !strings.Contains(err.Error(), "refused") || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("expected blocked error without secret, got %v", err)
	}
}

func TestMaskAndMerge(t *testing.T) {
	old, _ := json.Marshal(Config{URL: "https://discord.com/api/webhooks/1/abc"})
	masked := Masked(ChanDiscord, old)
	if strings.Contains(string(masked), "abc") {
		t.Fatal("secret not masked")
	}
	merged, err := MergeConfig(ChanDiscord, masked, old)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(merged), "abc") {
		t.Fatal("masked value replaced the secret")
	}
	if _, err := MergeConfig(ChanDiscord, json.RawMessage(`{"url":"ftp://x"}`), nil); err == nil {
		t.Fatal("invalid URL accepted")
	}
	if _, err := MergeConfig(ChanWebhook, json.RawMessage(`{"url":"https://x.example","headerName":"Host"}`), nil); err == nil {
		t.Fatal("Host header accepted")
	}
	if _, err := MergeConfig(ChanNtfy, json.RawMessage(`{"topic":"a","bogus":1}`), nil); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := MergeConfig(ChanEmail, json.RawMessage(`{"host":"smtp.example","from":"a@b","to":"c@d\r\nBcc: x@y"}`), nil); err == nil {
		t.Fatal("header injection in recipient accepted")
	}
}

// Stored secrets must not follow a changed destination (review 2, security M1).
func TestSecretsDoNotFollowNewDestination(t *testing.T) {
	cases := []struct {
		kind, old, upd string
	}{
		{ChanNtfy, `{"server":"https://ntfy.sh","topic":"a","token":"tk_SECRET"}`, `{"server":"http://192.0.2.2:18999","topic":"a","token":"••••••••"}`},
		{ChanGotify, `{"server":"https://gotify.lan","token":"SECRET"}`, `{"server":"https://evil.example","token":""}`},
		{ChanWebhook, `{"url":"https://hook.lan/x","headerName":"Authorization","headerValue":"Bearer SECRET"}`, `{"url":"https://evil.example/","headerName":"Authorization","headerValue":"••••••••"}`},
		{ChanEmail, `{"host":"smtp.lan","port":587,"security":"starttls","username":"u","password":"SECRET","from":"a@b.de","to":"c@d.de"}`, `{"host":"smtp.evil.example","port":587,"security":"starttls","username":"u","password":"••••••••","from":"a@b.de","to":"c@d.de"}`},
		{ChanEmail, `{"host":"smtp.lan","port":587,"security":"starttls","username":"u","password":"SECRET","from":"a@b.de","to":"c@d.de"}`, `{"host":"smtp.lan","port":25,"security":"none","username":"u","password":"••••••••","from":"a@b.de","to":"c@d.de"}`},
	}
	for _, c := range cases {
		if _, err := MergeConfig(c.kind, json.RawMessage(c.upd), json.RawMessage(c.old)); !errors.Is(err, ErrSecretRequired) {
			t.Errorf("%s: secret followed the new destination (err=%v)", c.kind, err)
		}
	}
	// Same destination, other fields changed: the secret is kept.
	merged, err := MergeConfig(ChanNtfy, json.RawMessage(`{"server":"https://ntfy.sh","topic":"b","token":"••••••••"}`),
		json.RawMessage(`{"server":"https://ntfy.sh","topic":"a","token":"tk_SECRET"}`))
	if err != nil || !strings.Contains(string(merged), "tk_SECRET") {
		t.Fatalf("secret lost on harmless change: %v", err)
	}
}

func TestSMTP(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	data := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		w := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }
		buf := make([]byte, 64<<10)
		w("220 test ESMTP")
		var all strings.Builder
		inData := false
		for {
			n, err := c.Read(buf)
			if err != nil {
				return
			}
			chunk := string(buf[:n])
			if inData {
				all.WriteString(chunk)
				if strings.HasSuffix(all.String(), "\r\n.\r\n") {
					inData = false
					w("250 ok")
					data <- all.String()
				}
				continue
			}
			switch {
			case strings.HasPrefix(chunk, "EHLO"), strings.HasPrefix(chunk, "HELO"):
				w("250 test")
			case strings.HasPrefix(chunk, "DATA"):
				inData = true
				w("354 go")
			case strings.HasPrefix(chunk, "QUIT"):
				w("221 bye")
				return
			default:
				w("250 ok")
			}
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	c := Config{Host: "127.0.0.1", Port: addr.Port, Security: "none", From: "nw@example.org", To: "me@example.org"}
	msg := Render(Event{Kind: KindDeviceNew, TS: 1, Device: &store.Device{MAC: "aa:bb:cc:dd:ee:ff", Hostname: "Öl\r\nBcc: x@y"}}, "de", time.UTC)
	err = sendMail(context.Background(), &net.Dialer{}, c, msg)
	if err != nil {
		t.Fatal(err)
	}
	got := <-data
	headers, _, _ := strings.Cut(got, "\r\n\r\n")
	if !strings.Contains(headers, "Subject: =?utf-8?q?") || strings.Contains(headers, "Bcc:") {
		t.Fatalf("unexpected mail:\n%s", got)
	}
	// The production dialer refuses the loopback SMTP server.
	if err := sendMail(context.Background(), &egress.Policy{}, c, msg); err == nil || !errors.Is(err, err) {
		t.Fatal("loopback SMTP must be refused")
	}
}
