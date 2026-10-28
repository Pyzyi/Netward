// Package fritzbox reads the host table of an AVM FRITZ!Box over TR-064.
//
// The router knows the names users gave their devices in its web UI and
// which phones are associated with its Wi-Fi even while they sleep and do
// not answer ARP. Netzwart uses both to name devices and to keep sleeping
// phones online.
//
// Security: authenticated calls go over HTTPS only. The FRITZ!Box uses a
// self-signed certificate, so the certificate is pinned on first use
// (SHA-256 of the leaf) and every later connection must present the same
// one. Credentials are sent with HTTP digest authentication, never in clear.
package fritzbox

import (
	"bytes"
	"context"
	"crypto/md5" // HTTP digest (RFC 2617) as the FRITZ!Box implements it requires MD5
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/Pyzyi/Netward/internal/textutil"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Errors reported to the user (see Status.Error).
var (
	ErrAuth        = errors.New("auth")        // wrong user/password or missing permission
	ErrCertChanged = errors.New("cert")        // pinned certificate no longer matches
	ErrUnreachable = errors.New("unreachable") // host or TR-064 not reachable
	ErrProtocol    = errors.New("protocol")    // unexpected answer
)

const (
	httpPort    = 49000
	defaultTLS  = 49443
	maxBody     = 4 << 20
	maxHosts    = 4000
	callTimeout = 10 * time.Second
)

// Host is one entry of the router's host table.
type Host struct {
	MAC    net.HardwareAddr
	IP     netip.Addr
	Name   string
	Active bool
	WLAN   bool
}

// DialFunc opens network connections (normally the egress policy dialer).
type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// Client talks to one FRITZ!Box.
type Client struct {
	host, user, pass string
	dial             DialFunc
	httpPort         int

	mu   sync.Mutex
	pin  []byte // pinned certificate hash; nil until the first connection
	seen []byte // hash presented by the last TLS handshake
	http *http.Client
}

// NewClient creates a client. pin is the certificate hash from an earlier
// session (nil to trust the first certificate).
func NewClient(host, user, pass string, pin []byte, dial DialFunc) *Client {
	c := &Client{host: host, user: user, pass: pass, dial: dial, httpPort: httpPort, pin: pin}
	tr := &http.Transport{
		Proxy:       nil,
		DialContext: dial,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// The certificate is self-signed; it is verified by pinning instead.
			InsecureSkipVerify: true, // replaced by VerifyPeerCertificate below
			VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
				if len(raw) == 0 {
					return ErrCertChanged
				}
				sum := sha256.Sum256(raw[0])
				c.mu.Lock()
				defer c.mu.Unlock()
				c.seen = sum[:]
				if c.pin != nil && subtle.ConstantTimeCompare(c.pin, sum[:]) != 1 {
					return ErrCertChanged
				}
				return nil
			},
		},
		TLSHandshakeTimeout:    callTimeout,
		ResponseHeaderTimeout:  callTimeout,
		MaxIdleConns:           2,
		IdleConnTimeout:        90 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
	}
	c.http = &http.Client{Transport: tr, Timeout: 2 * callTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return c
}

// Pin returns the certificate hash that is trusted after a successful call.
func (c *Client) Pin() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pin != nil {
		return c.pin
	}
	return c.seen
}

// Close releases idle connections.
func (c *Client) Close() { c.http.CloseIdleConnections() }

// Hosts returns the router's host table.
func (c *Client) Hosts(ctx context.Context) ([]Host, error) {
	base, err := c.secureBase(ctx)
	if err != nil {
		return nil, err
	}
	res, err := c.soap(ctx, base, "/upnp/control/hosts", "urn:dslforum-org:service:Hosts:1", "X_AVM-DE_GetHostListPath", nil)
	if err == nil {
		var hosts []Host
		if hosts, err = c.hostList(ctx, base, res["NewX_AVM-DE_HostListPath"]); err == nil {
			c.trust()
			return hosts, nil
		}
	}
	var fault *soapFault
	if !errors.As(err, &fault) {
		return nil, err
	}
	// Old firmware without the list action: query entry by entry.
	hosts, err := c.genericHosts(ctx, base)
	if err != nil {
		return nil, err
	}
	c.trust()
	return hosts, nil
}

// Firmware is what the router says about its own software.
type Firmware struct {
	Version          string // installed, e.g. "154.08.25"
	UpgradeAvailable bool
	NewVersion       string // offered update, if any
}

// Firmware reads the installed FRITZ!OS and whether an update is offered.
// Only these fields are taken from the answers: GetInfo also carries the
// serial number and the device log, which Netzwart neither stores nor logs.
func (c *Client) Firmware(ctx context.Context) (Firmware, error) {
	base, err := c.secureBase(ctx)
	if err != nil {
		return Firmware{}, err
	}
	info, err := c.soap(ctx, base, "/upnp/control/deviceinfo", "urn:dslforum-org:service:DeviceInfo:1", "GetInfo", nil)
	if err != nil {
		return Firmware{}, err
	}
	fw := Firmware{Version: textutil.Clean(info["NewSoftwareVersion"], 32)}
	// Without the update state the version alone would read as "up to date".
	ui, err := c.soap(ctx, base, "/upnp/control/userif", "urn:dslforum-org:service:UserInterface:1", "GetInfo", nil)
	if err != nil {
		return Firmware{}, err
	}
	fw.UpgradeAvailable = ui["NewUpgradeAvailable"] == "1"
	if fw.UpgradeAvailable {
		fw.NewVersion = textutil.Clean(ui["NewX_AVM-DE_Version"], 32)
	}
	return fw, nil
}

// trust pins the certificate that just served an authenticated call.
func (c *Client) trust() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pin == nil && c.seen != nil {
		c.pin = c.seen
	}
}

// secureBase asks for the HTTPS port (an unauthenticated call that carries
// no secrets) and returns the HTTPS base URL.
func (c *Client) secureBase(ctx context.Context) (*url.URL, error) {
	plain := &url.URL{Scheme: "http", Host: net.JoinHostPort(c.host, strconv.Itoa(c.httpPort))}
	port := defaultTLS
	res, err := c.call(ctx, plain, "/upnp/control/deviceinfo", "urn:dslforum-org:service:DeviceInfo:1", "GetSecurityPort", nil, false)
	if err != nil {
		var fault *soapFault
		if !errors.As(err, &fault) {
			return nil, err
		}
	} else if p, err := strconv.Atoi(res["NewSecurityPort"]); err == nil && p > 0 && p < 65536 {
		port = p
	}
	return &url.URL{Scheme: "https", Host: net.JoinHostPort(c.host, strconv.Itoa(port))}, nil
}

func (c *Client) soap(ctx context.Context, base *url.URL, control, service, action string, args [][2]string) (map[string]string, error) {
	return c.call(ctx, base, control, service, action, args, true)
}

func (c *Client) call(ctx context.Context, base *url.URL, control, service, action string, args [][2]string, auth bool) (map[string]string, error) {
	var body bytes.Buffer
	body.WriteString(`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:`)
	body.WriteString(action + ` xmlns:u="` + service + `">`)
	for _, a := range args {
		body.WriteString("<" + a[0] + ">")
		_ = xml.EscapeText(&body, []byte(a[1]))
		body.WriteString("</" + a[0] + ">")
	}
	body.WriteString(`</u:` + action + `></s:Body></s:Envelope>`)
	u := *base
	u.Path = control
	hdr := http.Header{
		"Content-Type": {`text/xml; charset="utf-8"`},
		"Soapaction":   {service + "#" + action},
	}
	status, data, err := c.do(ctx, http.MethodPost, u.String(), hdr, body.Bytes(), auth)
	if err != nil {
		return nil, err
	}
	fields, fault := parseSOAP(data)
	if fault != nil {
		if fault.Code == "606" { // UPnP "Action not authorized"; 401 means "Invalid Action"
			return nil, ErrAuth
		}
		return nil, fault
	}
	if status != http.StatusOK || fields == nil {
		return nil, fmt.Errorf("%w: HTTP %d", ErrProtocol, status)
	}
	return fields, nil
}

// do sends a request; on 401 it answers the digest challenge once.
func (c *Client) do(ctx context.Context, method, target string, hdr http.Header, body []byte, auth bool) (int, []byte, error) {
	send := func(authz string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		for k, v := range hdr {
			req.Header[k] = v
		}
		if authz != "" {
			req.Header.Set("Authorization", authz)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			if errors.Is(err, ErrCertChanged) {
				return nil, ErrCertChanged
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("%w: %v", ErrUnreachable, withoutURL(err))
		}
		return resp, nil
	}
	resp, err := send("")
	if err != nil {
		return 0, nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && auth {
		challenge := resp.Header.Get("WWW-Authenticate")
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		resp.Body.Close()
		if !strings.HasPrefix(target, "https://") {
			return 0, nil, ErrProtocol // never send credentials over plain HTTP
		}
		authz, err := digest(challenge, method, target, c.user, c.pass)
		if err != nil {
			return 0, nil, err
		}
		if resp, err = send(authz); err != nil {
			return 0, nil, err
		}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", ErrUnreachable, withoutURL(err))
	}
	if len(data) > maxBody {
		return 0, nil, fmt.Errorf("%w: response too large", ErrProtocol)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return 0, nil, ErrAuth
	}
	return resp.StatusCode, data, nil
}

// digest answers an RFC 2617 challenge (MD5, qop=auth).
func digest(challenge, method, target, user, pass string) (string, error) {
	if !strings.HasPrefix(strings.ToLower(challenge), "digest ") {
		return "", fmt.Errorf("%w: unsupported authentication", ErrProtocol)
	}
	p := parseChallenge(challenge[7:])
	if p["nonce"] == "" || (p["algorithm"] != "" && !strings.EqualFold(p["algorithm"], "MD5")) {
		return "", fmt.Errorf("%w: unsupported digest challenge", ErrProtocol)
	}
	u, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	uri := u.RequestURI()
	h := func(s string) string { sum := md5.Sum([]byte(s)); return hex.EncodeToString(sum[:]) }
	ha1 := h(user + ":" + p["realm"] + ":" + pass)
	ha2 := h(method + ":" + uri)
	var cb [8]byte
	_, _ = rand.Read(cb[:])
	cnonce, nc := hex.EncodeToString(cb[:]), "00000001"
	var resp, qop string
	if qops := strings.Split(p["qop"], ","); p["qop"] != "" {
		for _, q := range qops {
			if strings.TrimSpace(q) == "auth" {
				qop = "auth"
			}
		}
		if qop == "" {
			return "", fmt.Errorf("%w: unsupported digest qop", ErrProtocol)
		}
		resp = h(ha1 + ":" + p["nonce"] + ":" + nc + ":" + cnonce + ":" + qop + ":" + ha2)
	} else {
		resp = h(ha1 + ":" + p["nonce"] + ":" + ha2)
	}
	q := func(s string) string { return strconv.Quote(s) }
	out := "Digest username=" + q(user) + ", realm=" + q(p["realm"]) + ", nonce=" + q(p["nonce"]) +
		", uri=" + q(uri) + ", algorithm=MD5, response=" + q(resp)
	if qop != "" {
		out += ", qop=" + qop + ", nc=" + nc + ", cnonce=" + q(cnonce)
	}
	if p["opaque"] != "" {
		out += ", opaque=" + q(p["opaque"])
	}
	return out, nil
}

// parseChallenge splits `k=v, k="v, w"` into a map.
func parseChallenge(s string) map[string]string {
	out := map[string]string{}
	for s = strings.TrimSpace(s); s != ""; s = strings.TrimLeft(s, " ,\t") {
		eq := strings.IndexByte(s, '=')
		if eq <= 0 {
			break
		}
		key := strings.ToLower(strings.TrimSpace(s[:eq]))
		s = strings.TrimSpace(s[eq+1:])
		var val string
		if strings.HasPrefix(s, `"`) {
			end := 1
			for end < len(s) && s[end] != '"' {
				if s[end] == '\\' {
					end++
				}
				end++
			}
			val = strings.ReplaceAll(s[1:min(end, len(s))], `\"`, `"`)
			s = s[min(end+1, len(s)):]
		} else if c := strings.IndexByte(s, ','); c >= 0 {
			val, s = strings.TrimSpace(s[:c]), s[c:]
		} else {
			val, s = strings.TrimSpace(s), ""
		}
		out[key] = val
	}
	return out
}

// soapFault is a UPnP error answer.
type soapFault struct{ Code, Desc string }

func (f *soapFault) Error() string { return "TR-064 error " + f.Code + " " + f.Desc }

// parseSOAP returns the leaf elements of the response body by local name.
func parseSOAP(data []byte) (map[string]string, *soapFault) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	fields := map[string]string{}
	var stack []string
	var text strings.Builder
	inBody := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			text.Reset()
			if t.Name.Local == "Body" {
				inBody = true
			}
		case xml.CharData:
			if text.Len() < 4096 {
				text.Write(t)
			}
		case xml.EndElement:
			if inBody && len(stack) > 0 && len(fields) < 256 {
				fields[stack[len(stack)-1]] = strings.TrimSpace(text.String())
			}
			text.Reset()
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if !inBody {
		return nil, nil
	}
	if code, ok := fields["errorCode"]; ok {
		return nil, &soapFault{Code: code, Desc: fields["errorDescription"]}
	}
	if _, ok := fields["faultcode"]; ok {
		return nil, &soapFault{Code: "fault", Desc: fields["faultstring"]}
	}
	return fields, nil
}

// hostList downloads the XML host list the router offers at path.
func (c *Client) hostList(ctx context.Context, base *url.URL, path string) ([]Host, error) {
	ref, err := url.Parse(path)
	// Only a path on the same router is acceptable.
	if err != nil || ref.Scheme != "" || ref.Host != "" || ref.User != nil || !strings.HasPrefix(ref.Path, "/") || strings.HasPrefix(ref.Path, "//") {
		return nil, fmt.Errorf("%w: invalid host list path", ErrProtocol)
	}
	u := *base
	u.Path, u.RawQuery = ref.Path, ref.RawQuery
	status, data, err := c.do(ctx, http.MethodGet, u.String(), nil, nil, true)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%w: host list HTTP %d", ErrProtocol, status)
	}
	var doc struct {
		Items []struct {
			IP        string `xml:"IPAddress"`
			MAC       string `xml:"MACAddress"`
			Active    string `xml:"Active"`
			HostName  string `xml:"HostName"`
			Interface string `xml:"InterfaceType"`
		} `xml:"Item"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%w: host list: %v", ErrProtocol, err)
	}
	var out []Host
	for _, it := range doc.Items {
		if h, ok := makeHost(it.MAC, it.IP, it.HostName, it.Active, it.Interface); ok && len(out) < maxHosts {
			out = append(out, h)
		}
	}
	return out, nil
}

func (c *Client) genericHosts(ctx context.Context, base *url.URL) ([]Host, error) {
	const svc = "urn:dslforum-org:service:Hosts:1"
	res, err := c.soap(ctx, base, "/upnp/control/hosts", svc, "GetHostNumberOfEntries", nil)
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(res["NewHostNumberOfEntries"])
	if err != nil || n < 0 {
		return nil, fmt.Errorf("%w: host count", ErrProtocol)
	}
	var out []Host
	for i := 0; i < min(n, 512); i++ {
		r, err := c.soap(ctx, base, "/upnp/control/hosts", svc, "GetGenericHostEntry", [][2]string{{"NewIndex", strconv.Itoa(i)}})
		if err != nil {
			return nil, err
		}
		if h, ok := makeHost(r["NewMACAddress"], r["NewIPAddress"], r["NewHostName"], r["NewActive"], r["NewInterfaceType"]); ok {
			out = append(out, h)
		}
	}
	return out, nil
}

func makeHost(mac, ip, name, active, iface string) (Host, bool) {
	hw, err := net.ParseMAC(strings.TrimSpace(mac))
	if err != nil || len(hw) != 6 {
		return Host{}, false
	}
	h := Host{MAC: hw, Name: strings.TrimSpace(name), Active: strings.TrimSpace(active) == "1",
		WLAN: strings.TrimSpace(iface) == "802.11"}
	if a, err := netip.ParseAddr(strings.TrimSpace(ip)); err == nil && a.Is4() {
		h.IP = a
	}
	if GenericName(h.Name) {
		h.Name = ""
	}
	return h, true
}

// GenericName reports names the FRITZ!Box invents for unnamed devices
// ("PC-192-168-178-23") which carry no information.
func GenericName(s string) bool {
	if s == "" {
		return true
	}
	lower := strings.ToLower(s)
	for _, p := range []string{"pc-", "android-", "unknown"} {
		if strings.HasPrefix(lower, p) {
			rest := strings.TrimPrefix(lower, p)
			digits := 0
			for _, r := range rest {
				if r >= '0' && r <= '9' || r == '-' || r >= 'a' && r <= 'f' {
					digits++
				}
			}
			if digits == len(rest) {
				return true
			}
		}
	}
	return false
}

// withoutURL drops the request URL from an error: the paths the FRITZ!Box
// hands out carry its session ID ("…?sid=…"), which must not reach the log.
func withoutURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}
