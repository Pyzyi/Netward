package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Pyzyi/netzwart/internal/egress"
)

// Kinds of channels.
const (
	ChanNtfy     = "ntfy"
	ChanGotify   = "gotify"
	ChanPushover = "pushover"
	ChanTelegram = "telegram"
	ChanDiscord  = "discord"
	ChanSlack    = "slack"
	ChanWebhook  = "webhook"
	ChanEmail    = "email"
)

// Config is the union of all channel settings. Which fields apply depends
// on the kind; secrets are listed in secretFields.
type Config struct {
	Server      string `json:"server,omitempty"` // ntfy, gotify
	Topic       string `json:"topic,omitempty"`  // ntfy
	Token       string `json:"token,omitempty"`  // ntfy, gotify, pushover app token, telegram bot token (secret)
	User        string `json:"user,omitempty"`   // pushover user key (secret)
	ChatID      string `json:"chatId,omitempty"` // telegram
	URL         string `json:"url,omitempty"`    // discord, slack, webhook (secret: contains credentials)
	HeaderName  string `json:"headerName,omitempty"`
	HeaderValue string `json:"headerValue,omitempty"` // secret
	Host        string `json:"host,omitempty"`        // email
	Port        int    `json:"port,omitempty"`
	Security    string `json:"security,omitempty"` // starttls | tls | none
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"` // secret
	From        string `json:"from,omitempty"`
	To          string `json:"to,omitempty"`
	Priority    int    `json:"priority,omitempty"`
}

var secretFields = map[string][]string{
	ChanNtfy:     {"token"},
	ChanGotify:   {"token"},
	ChanPushover: {"token", "user"},
	ChanTelegram: {"token"},
	ChanDiscord:  {"url"},
	ChanSlack:    {"url"},
	ChanWebhook:  {"url", "headerValue"},
	ChanEmail:    {"password"},
}

// Kinds lists the supported channel kinds.
func Kinds() []string {
	return []string{ChanNtfy, ChanGotify, ChanPushover, ChanTelegram, ChanDiscord, ChanSlack, ChanWebhook, ChanEmail}
}

// ValidKind reports whether k is supported.
func ValidKind(k string) bool { _, ok := secretFields[k]; return ok }

const mask = "••••••••"

func (c *Config) secretPtr(field string) *string {
	switch field {
	case "token":
		return &c.Token
	case "user":
		return &c.User
	case "url":
		return &c.URL
	case "headerValue":
		return &c.HeaderValue
	case "password":
		return &c.Password
	}
	return nil
}

// Masked returns the config with secrets replaced by a mask.
func Masked(kind string, raw json.RawMessage) json.RawMessage {
	var c Config
	_ = json.Unmarshal(raw, &c)
	for _, f := range secretFields[kind] {
		if p := c.secretPtr(f); p != nil && *p != "" {
			*p = mask
		}
	}
	out, _ := json.Marshal(c)
	return out
}

// MergeConfig keeps old secrets where the update sends an empty or masked value.
func MergeConfig(kind string, update, old json.RawMessage) (json.RawMessage, error) {
	var u, o Config
	dec := json.NewDecoder(bytes.NewReader(update))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&u); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	if len(old) > 0 {
		_ = json.Unmarshal(old, &o)
	}
	for _, f := range secretFields[kind] {
		p, q := u.secretPtr(f), o.secretPtr(f)
		if *p == "" || *p == mask {
			*p = *q
		}
	}
	if err := u.validate(kind); err != nil {
		return nil, err
	}
	out, _ := json.Marshal(u)
	return out, nil
}

func httpURL(s string, required bool) error {
	if s == "" {
		if required {
			return errors.New("URL is required")
		}
		return nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return errors.New("URL must start with http:// or https:// and must not contain credentials")
	}
	return nil
}

func short(s string, max int, field string) error {
	if utf8.RuneCountInString(s) > max || strings.ContainsAny(s, "\r\n\x00") {
		return fmt.Errorf("%s is invalid or too long", field)
	}
	return nil
}

func (c Config) validate(kind string) error {
	var errs []error
	add := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}
	for _, f := range []struct{ v, n string }{{c.Topic, "topic"}, {c.Token, "token"}, {c.User, "user"},
		{c.ChatID, "chatId"}, {c.HeaderName, "headerName"}, {c.HeaderValue, "headerValue"}, {c.Host, "host"},
		{c.Username, "username"}, {c.Password, "password"}, {c.From, "from"}, {c.To, "to"}} {
		add(short(f.v, 512, f.n))
	}
	add(short(c.URL, 2048, "url"))
	add(short(c.Server, 512, "server"))
	switch kind {
	case ChanNtfy:
		add(httpURL(c.Server, false))
		if c.Topic == "" || strings.ContainsAny(c.Topic, "/?#") {
			add(errors.New("topic is required and may not contain / ? #"))
		}
	case ChanGotify:
		add(httpURL(c.Server, true))
		if c.Token == "" {
			add(errors.New("token is required"))
		}
	case ChanPushover:
		if c.Token == "" || c.User == "" {
			add(errors.New("app token and user key are required"))
		}
	case ChanTelegram:
		if c.Token == "" || c.ChatID == "" || strings.ContainsAny(c.Token, "/?#") {
			add(errors.New("bot token and chat ID are required"))
		}
	case ChanDiscord, ChanSlack, ChanWebhook:
		add(httpURL(c.URL, true))
		if c.HeaderName != "" && !validHeaderName(c.HeaderName) {
			add(errors.New("invalid header name"))
		}
	case ChanEmail:
		if c.Host == "" || c.From == "" || c.To == "" {
			add(errors.New("SMTP server, sender and recipient are required"))
		}
		if c.Port < 0 || c.Port > 65535 {
			add(errors.New("invalid port"))
		}
		if c.Security != "" && c.Security != "starttls" && c.Security != "tls" && c.Security != "none" {
			add(errors.New("security must be starttls, tls or none"))
		}
		for _, a := range append(strings.Split(c.To, ","), c.From) {
			if !strings.Contains(a, "@") || strings.ContainsAny(a, "<>\r\n") {
				add(errors.New("invalid e-mail address"))
				break
			}
		}
	default:
		add(errors.New("unknown channel kind"))
	}
	return errors.Join(errs...)
}

func validHeaderName(h string) bool {
	if len(h) > 64 {
		return false
	}
	for _, r := range h {
		if !(r == '-' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	lower := strings.ToLower(h)
	return lower != "host" && lower != "content-length" && lower != "transfer-encoding" && lower != "connection"
}

// SendError never contains URLs or tokens, only a short reason.
type SendError struct{ Reason string }

func (e *SendError) Error() string { return e.Reason }

func summarize(err error) error {
	var be *egress.BlockedError
	var ue *url.Error
	var ne net.Error
	switch {
	case errors.As(err, &be):
		return &SendError{Reason: be.Error()}
	case errors.As(err, &ne) && ne.Timeout():
		return &SendError{Reason: "timeout"}
	case errors.As(err, &ue):
		var oe *net.OpError
		if errors.As(ue.Err, &oe) {
			return &SendError{Reason: "connection failed: " + oe.Err.Error()}
		}
		var dnsErr *net.DNSError
		if errors.As(ue.Err, &dnsErr) {
			return &SendError{Reason: "host not found"}
		}
		return &SendError{Reason: "request failed"}
	}
	var se *SendError
	if errors.As(err, &se) {
		return se
	}
	return &SendError{Reason: "delivery failed"}
}

func post(ctx context.Context, client *http.Client, method, u string, body io.Reader, contentType string, headers map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return &SendError{Reason: "invalid URL"}
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "Netzwart")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return summarize(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &SendError{Reason: "HTTP " + strconv.Itoa(resp.StatusCode)}
	}
	return nil
}

func jsonBody(v any) io.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

// discordEscape neutralises Markdown so device names cannot create links,
// mentions or formatting.
func discordEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`\*_~|>[]()#:-`+"`@", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// slackEscape escapes the three characters Slack's mrkdwn treats specially;
// this also defuses <!channel> and <url|text> constructs.
func slackEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func ntfyPriority(sev string, override int) int {
	if override >= 1 && override <= 5 {
		return override
	}
	switch sev {
	case "critical":
		return 5
	case "warning":
		return 4
	}
	return 3
}

// Send delivers msg through a channel of the given kind.
func Send(ctx context.Context, client *http.Client, dialer *egress.Policy, kind string, raw json.RawMessage, msg Message) error {
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return &SendError{Reason: "invalid configuration"}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	switch kind {
	case ChanNtfy:
		server := strings.TrimRight(c.Server, "/")
		if server == "" {
			server = "https://ntfy.sh"
		}
		h := map[string]string{}
		if c.Token != "" {
			h["Authorization"] = "Bearer " + c.Token
		}
		// JSON publishing keeps arbitrary UTF-8 out of HTTP headers.
		return post(ctx, client, http.MethodPost, server, jsonBody(map[string]any{
			"topic": c.Topic, "title": msg.Title, "message": msg.Body,
			"priority": ntfyPriority(msg.Severity, c.Priority), "tags": []string{"satellite_antenna"},
		}), "application/json", h)
	case ChanGotify:
		return post(ctx, client, http.MethodPost, strings.TrimRight(c.Server, "/")+"/message",
			jsonBody(map[string]any{"title": msg.Title, "message": msg.Body, "priority": ntfyPriority(msg.Severity, c.Priority) + 2}),
			"application/json", map[string]string{"X-Gotify-Key": c.Token})
	case ChanPushover:
		form := url.Values{"token": {c.Token}, "user": {c.User}, "title": {msg.Title}, "message": {msg.Body}}
		if msg.Severity == "critical" {
			form.Set("priority", "1")
		}
		return post(ctx, client, http.MethodPost, "https://api.pushover.net/1/messages.json",
			strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", nil)
	case ChanTelegram:
		// No parse_mode: the text is delivered verbatim, nothing is interpreted.
		return post(ctx, client, http.MethodPost, "https://api.telegram.org/bot"+c.Token+"/sendMessage",
			jsonBody(map[string]any{"chat_id": c.ChatID, "text": msg.Title + "\n" + msg.Body, "disable_web_page_preview": true}),
			"application/json", nil)
	case ChanDiscord:
		return post(ctx, client, http.MethodPost, c.URL, jsonBody(map[string]any{
			"username": "Netzwart", "content": "**" + discordEscape(msg.Title) + "**\n" + discordEscape(msg.Body),
			"allowed_mentions": map[string]any{"parse": []string{}},
		}), "application/json", nil)
	case ChanSlack:
		return post(ctx, client, http.MethodPost, c.URL, jsonBody(map[string]any{
			"text": "*" + slackEscape(msg.Title) + "*\n" + slackEscape(msg.Body), "unfurl_links": false, "unfurl_media": false,
		}), "application/json", nil)
	case ChanWebhook:
		payload := map[string]any{"event": msg.Kind, "severity": msg.Severity, "title": msg.Title,
			"message": msg.Body, "timestamp": time.UnixMilli(msg.TS).UTC().Format(time.RFC3339)}
		if msg.Device != nil {
			payload["device"] = map[string]any{"id": msg.Device.ID, "name": msg.Device.Name, "hostname": msg.Device.Hostname,
				"ip": msg.Device.IP, "mac": msg.Device.MAC, "vendor": msg.Device.Vendor, "type": msg.Device.Type}
		}
		if len(msg.Data) > 0 {
			payload["data"] = msg.Data
		}
		h := map[string]string{}
		if c.HeaderName != "" {
			h[c.HeaderName] = c.HeaderValue
		}
		return post(ctx, client, http.MethodPost, c.URL, jsonBody(payload), "application/json", h)
	case ChanEmail:
		return sendMail(ctx, dialer, c, msg)
	}
	return &SendError{Reason: "unknown channel kind"}
}

func headerSafe(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r < 0x20 {
			return ' '
		}
		return r
	}, s)
}

// contextDialer is satisfied by *egress.Policy.
type contextDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

func sendMail(ctx context.Context, dialer contextDialer, c Config, msg Message) error {
	port := c.Port
	sec := c.Security
	if sec == "" {
		sec = "starttls"
	}
	if port == 0 {
		port = map[string]int{"starttls": 587, "tls": 465, "none": 25}[sec]
	}
	addr := net.JoinHostPort(c.Host, strconv.Itoa(port))
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return summarize(err)
	}
	if d, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(d)
	}
	tlsConf := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
	if sec == "tls" {
		conn = tls.Client(conn, tlsConf)
	}
	cl, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		conn.Close()
		return &SendError{Reason: "SMTP handshake failed"}
	}
	defer cl.Close()
	if sec == "starttls" {
		if err := cl.StartTLS(tlsConf); err != nil {
			return &SendError{Reason: "STARTTLS failed"}
		}
	}
	if c.Username != "" {
		if err := cl.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host)); err != nil {
			return &SendError{Reason: "SMTP authentication failed"}
		}
	}
	if err := cl.Mail(c.From); err != nil {
		return &SendError{Reason: "sender rejected"}
	}
	var to []string
	for _, a := range strings.Split(c.To, ",") {
		if a = strings.TrimSpace(a); a != "" {
			if err := cl.Rcpt(a); err != nil {
				return &SendError{Reason: "recipient rejected"}
			}
			to = append(to, a)
		}
	}
	w, err := cl.Data()
	if err != nil {
		return &SendError{Reason: "SMTP DATA failed"}
	}
	var b strings.Builder
	b.WriteString("From: Netzwart <" + headerSafe(c.From) + ">\r\n")
	b.WriteString("To: " + headerSafe(strings.Join(to, ", ")) + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", headerSafe(msg.Title)) + "\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	body := strings.ReplaceAll(msg.Body, "\r", "")
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, ".") {
			line = "." + line
		}
		b.WriteString(line + "\r\n")
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return &SendError{Reason: "SMTP write failed"}
	}
	if err := w.Close(); err != nil {
		return &SendError{Reason: "message rejected"}
	}
	return cl.Quit()
}
