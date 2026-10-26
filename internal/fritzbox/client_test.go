package fritzbox

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	user, pass = "netzwart", "geheim-123"
	realm      = "F!Box SOAP-Auth"
	nonce      = "4F2D1A8B9C"
)

func soapResponse(action string, fields map[string]string) string {
	var b strings.Builder
	for k, v := range fields {
		fmt.Fprintf(&b, "<%s>%s</%s>", k, v, k)
	}
	return `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:` + action +
		`Response xmlns:u="urn:x">` + b.String() + `</u:` + action + `Response></s:Body></s:Envelope>`
}

const fault = `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><faultcode>s:Client</faultcode><faultstring>UPnPError</faultstring><detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0"><errorCode>401</errorCode><errorDescription>Invalid Action</errorDescription></UPnPError></detail></s:Fault></s:Body></s:Envelope>`

// checkDigest verifies the Authorization header like the FRITZ!Box does.
func checkDigest(r *http.Request, password string) bool {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Digest ") {
		return false
	}
	p := parseChallenge(h[7:])
	md := func(s string) string { x := md5.Sum([]byte(s)); return hex.EncodeToString(x[:]) }
	ha1 := md(p["username"] + ":" + realm + ":" + password)
	ha2 := md(r.Method + ":" + p["uri"])
	want := md(ha1 + ":" + nonce + ":" + p["nc"] + ":" + p["cnonce"] + ":" + p["qop"] + ":" + ha2)
	return p["username"] == user && p["nonce"] == nonce && p["uri"] == r.URL.RequestURI() && p["response"] == want
}

type fakeBox struct {
	plain, secure *httptest.Server
	listAction    bool // supports X_AVM-DE_GetHostListPath
	plainAuth     atomic.Int32
}

func newFakeBox(t *testing.T, listAction bool) *fakeBox {
	f := &fakeBox{listAction: listAction}
	f.secure = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/devicehostlist.lua" {
			if r.URL.Query().Get("sid") != "abc" {
				w.WriteHeader(403)
				return
			}
			io.WriteString(w, `<List><ItemCount>3</ItemCount>
<Item><Index>1</Index><IPAddress>192.168.178.20</IPAddress><MACAddress>AA:BB:CC:00:00:01</MACAddress><Active>1</Active><HostName>Annas-iPhone</HostName><InterfaceType>802.11</InterfaceType></Item>
<Item><Index>2</Index><IPAddress>192.168.178.21</IPAddress><MACAddress>AA:BB:CC:00:00:02</MACAddress><Active>0</Active><HostName>PC-192-168-178-21</HostName><InterfaceType>Ethernet</InterfaceType></Item>
<Item><Index>3</Index><IPAddress></IPAddress><MACAddress>garbage</MACAddress><Active>1</Active><HostName>x</HostName></Item>
</List>`)
			return
		}
		if !checkDigest(r, pass) {
			w.Header().Set("WWW-Authenticate", `Digest realm="`+realm+`", nonce="`+nonce+`", algorithm=MD5, qop="auth"`)
			w.WriteHeader(401)
			return
		}
		action := r.Header.Get("Soapaction")
		body, _ := io.ReadAll(r.Body)
		switch {
		case strings.HasSuffix(action, "#X_AVM-DE_GetHostListPath") && f.listAction:
			io.WriteString(w, soapResponse("X_AVM-DE_GetHostListPath", map[string]string{"NewX_AVM-DE_HostListPath": "/devicehostlist.lua?sid=abc"}))
		case strings.HasSuffix(action, "#GetInfo") && strings.Contains(action, "DeviceInfo"):
			io.WriteString(w, soapResponse("GetInfo", map[string]string{"NewSoftwareVersion": "154.07.57",
				"NewSerialNumber": "SECRET-SERIAL", "NewDeviceLog": "secret log"}))
		case strings.HasSuffix(action, "#GetInfo") && strings.Contains(action, "UserInterface"):
			io.WriteString(w, soapResponse("GetInfo", map[string]string{"NewUpgradeAvailable": "1", "NewX_AVM-DE_Version": "154.08.25"}))
		case strings.HasSuffix(action, "#GetHostNumberOfEntries"):
			io.WriteString(w, soapResponse("GetHostNumberOfEntries", map[string]string{"NewHostNumberOfEntries": "1"}))
		case strings.HasSuffix(action, "#GetGenericHostEntry") && strings.Contains(string(body), "<NewIndex>0</NewIndex>"):
			io.WriteString(w, soapResponse("GetGenericHostEntry", map[string]string{"NewMACAddress": "AA:BB:CC:00:00:09",
				"NewIPAddress": "192.168.178.9", "NewHostName": "Drucker &amp; Scanner", "NewActive": "1", "NewInterfaceType": "Ethernet"}))
		default:
			w.WriteHeader(500)
			io.WriteString(w, fault)
		}
	}))
	_, port, _ := net.SplitHostPort(f.secure.Listener.Addr().String())
	f.plain = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			f.plainAuth.Add(1)
		}
		if strings.HasSuffix(r.Header.Get("Soapaction"), "#GetSecurityPort") {
			io.WriteString(w, soapResponse("GetSecurityPort", map[string]string{"NewSecurityPort": port}))
			return
		}
		w.Header().Set("WWW-Authenticate", `Digest realm="`+realm+`", nonce="`+nonce+`", qop="auth"`)
		w.WriteHeader(401)
	}))
	t.Cleanup(func() { f.plain.Close(); f.secure.Close() })
	return f
}

func (f *fakeBox) client(password string, pin []byte) *Client {
	c := NewClient("127.0.0.1", user, password, pin, (&net.Dialer{}).DialContext)
	_, port, _ := net.SplitHostPort(f.plain.Listener.Addr().String())
	fmt.Sscan(port, &c.httpPort)
	return c
}

func TestHostListAndPinning(t *testing.T) {
	f := newFakeBox(t, true)
	c := f.client(pass, nil)
	hosts, err := c.Hosts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 || hosts[0].Name != "Annas-iPhone" || !hosts[0].Active || !hosts[0].WLAN ||
		hosts[1].Name != "" || hosts[1].Active || hosts[1].IP.String() != "192.168.178.21" {
		t.Fatalf("unexpected hosts %+v", hosts)
	}
	pin := c.Pin()
	if len(pin) != 32 {
		t.Fatal("certificate not pinned")
	}
	if f.plainAuth.Load() != 0 {
		t.Fatal("credentials were sent over plain HTTP")
	}
	// Same certificate: fine. Different pin: refused before any credentials are sent.
	if _, err := f.client(pass, pin).Hosts(context.Background()); err != nil {
		t.Fatal(err)
	}
	other := make([]byte, 32)
	if _, err := f.client(pass, other).Hosts(context.Background()); !errors.Is(err, ErrCertChanged) {
		t.Fatalf("changed certificate accepted: %v", err)
	}
}

func TestWrongPassword(t *testing.T) {
	f := newFakeBox(t, true)
	c := f.client("falsch", nil)
	if _, err := c.Hosts(context.Background()); !errors.Is(err, ErrAuth) {
		t.Fatalf("got %v, want ErrAuth", err)
	}
	if c.Pin() == nil || c.pin != nil {
		t.Fatal("certificate must not be trusted after a failed login")
	}
}

func TestGenericFallback(t *testing.T) {
	f := newFakeBox(t, false)
	hosts, err := f.client(pass, nil).Hosts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Name != "Drucker & Scanner" || hosts[0].WLAN {
		t.Fatalf("unexpected %+v", hosts)
	}
}

func TestUnreachable(t *testing.T) {
	c := NewClient("127.0.0.1", user, pass, nil, (&net.Dialer{}).DialContext)
	c.httpPort = 1
	if _, err := c.Hosts(context.Background()); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("got %v", err)
	}
}

func TestHostListPathMustStayOnRouter(t *testing.T) {
	c := NewClient("127.0.0.1", user, pass, nil, (&net.Dialer{}).DialContext)
	base := mustURL("https://127.0.0.1:49443")
	for _, p := range []string{"https://evil.example/x", "//evil.example/x", "relative", "http://user@x/"} {
		if _, err := c.hostList(context.Background(), base, p); !errors.Is(err, ErrProtocol) {
			t.Errorf("%q accepted (%v)", p, err)
		}
	}
}

func TestDigestRefusesPlainHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("credentials sent over HTTP")
		}
		w.Header().Set("WWW-Authenticate", `Digest realm="x", nonce="y", qop="auth"`)
		w.WriteHeader(401)
	}))
	defer srv.Close()
	c := NewClient("127.0.0.1", user, pass, nil, (&net.Dialer{}).DialContext)
	if _, _, err := c.do(context.Background(), "POST", srv.URL+"/upnp/control/hosts", nil, nil, true); !errors.Is(err, ErrProtocol) {
		t.Fatalf("got %v", err)
	}
}

func TestParseChallenge(t *testing.T) {
	p := parseChallenge(`realm="F!Box, SOAP", nonce="a\"b", algorithm=MD5, qop="auth,auth-int"`)
	if p["realm"] != "F!Box, SOAP" || p["nonce"] != `a"b` || p["algorithm"] != "MD5" || p["qop"] != "auth,auth-int" {
		t.Fatalf("%#v", p)
	}
}

func TestGenericName(t *testing.T) {
	for s, want := range map[string]bool{"PC-192-168-178-23": true, "android-3fa2b1c9d0e1f2a3": true, "": true,
		"Annas-iPhone": false, "PC-Arbeitszimmer": false, "Unknown": true} {
		if GenericName(s) != want {
			t.Errorf("GenericName(%q) != %v", s, want)
		}
	}
}

func FuzzParseSOAP(f *testing.F) {
	f.Add([]byte(fault))
	f.Add([]byte(soapResponse("X", map[string]string{"A": "b"})))
	f.Fuzz(func(t *testing.T, data []byte) { parseSOAP(data) })
}

func FuzzParseChallenge(f *testing.F) {
	f.Add(`realm="x", nonce="y\"", qop=auth`)
	f.Fuzz(func(t *testing.T, s string) { parseChallenge(s) })
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func TestFirmware(t *testing.T) {
	f := newFakeBox(t, true)
	fw, err := f.client(pass, nil).Firmware(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if fw.Version != "154.07.57" || !fw.UpgradeAvailable || fw.NewVersion != "154.08.25" {
		t.Fatalf("unexpected firmware %+v", fw)
	}
	if strings.Contains(fmt.Sprintf("%+v", fw), "SECRET") {
		t.Fatal("serial number kept")
	}
}
