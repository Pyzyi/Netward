package egress

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestCheck(t *testing.T) {
	p := &Policy{}
	blocked := []string{"127.0.0.1", "127.8.9.10", "::1", "0.0.0.0", "0.1.2.3", "::", "169.254.169.254",
		"fe80::1", "224.0.0.251", "ff02::1", "255.255.255.255", "::ffff:127.0.0.1"}
	for _, s := range blocked {
		if err := p.Check(netip.MustParseAddr(s)); !errors.Is(err, ErrBlocked) {
			t.Errorf("%s should be blocked, got %v", s, err)
		}
	}
	allowed := []string{"192.168.1.20", "10.0.0.5", "1.1.1.1", "2001:db8::1", "100.64.1.1"}
	for _, s := range allowed {
		if err := p.Check(netip.MustParseAddr(s)); err != nil {
			t.Errorf("%s should be allowed, got %v", s, err)
		}
	}
	p.SetDenied([]netip.Addr{netip.MustParseAddr("192.168.1.20")})
	if err := p.Check(netip.MustParseAddr("192.168.1.20")); err == nil {
		t.Error("explicitly denied address allowed")
	}
}

func TestHTTPClientBlocksLoopbackAndRedirects(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()
	c := (&Policy{}).HTTPClient(2*time.Second, 3)
	for _, u := range []string{srv.URL, "http://localhost:" + srv.Listener.Addr().(*net.TCPAddr).AddrPort().String()[len("127.0.0.1:"):]} {
		_, err := c.Get(u)
		if !errors.Is(err, ErrBlocked) {
			t.Fatalf("GET %s: want ErrBlocked, got %v", u, err)
		}
	}
	if hit {
		t.Fatal("request reached the loopback server")
	}
}

func TestDialContextBlocks(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, err = (&Policy{}).DialContext(context.Background(), "tcp", ln.Addr().String())
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("want ErrBlocked, got %v", err)
	}
}
