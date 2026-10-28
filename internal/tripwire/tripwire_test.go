package tripwire

import (
	"io"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type addr string

func (a addr) Network() string { return "tcp" }
func (a addr) String() string  { return string(a) }

func TestNoteFilters(t *testing.T) {
	var got []Hit
	own := netip.MustParseAddr("192.168.1.5")
	m := New(quietLog(), nil, func(ip netip.Addr) bool { return ip == own }, func(h Hit) { got = append(got, h) })
	m.Configure(true, nil, []netip.Prefix{netip.MustParsePrefix("192.168.1.200/32")})
	now := time.Unix(1_800_000_000, 0)
	m.now = func() time.Time { return now }

	for _, a := range []string{
		"127.0.0.1:5000",     // loopback
		"192.168.1.5:5000",   // the host itself
		"192.168.1.200:5000", // allowed
		"8.8.8.8:5000",       // from the internet
		"192.168.1.42:5000",  // reported
		"192.168.1.42:5001",  // same address and port: quiet
		"[fd00::7]:5000",     // ULA: reported
		"100.101.1.2:5000",   // Tailscale: reported
	} {
		m.note(addr(a), 2323, "telnet")
	}
	if len(got) != 3 || got[0].IP.String() != "192.168.1.42" || got[1].IP.String() != "fd00::7" || got[2].IP.String() != "100.101.1.2" {
		t.Fatalf("hits = %+v", got)
	}
	m.note(addr("192.168.1.42:5000"), 3389, "rdp") // another port counts
	now = now.Add(quiet)
	m.note(addr("192.168.1.42:5000"), 2323, "telnet") // after the quiet time again
	if len(got) != 5 {
		t.Fatalf("hits = %d, want 5", len(got))
	}
	if st := m.Status(); st.LastHit == nil || st.LastHit.Port != 2323 {
		t.Fatalf("last hit = %+v", st.LastHit)
	}
}

func TestDecoyGreetsAndReports(t *testing.T) {
	var mu sync.Mutex
	hits := make(chan Hit, 4)
	m := New(quietLog(), nil, nil, func(h Hit) { hits <- h })
	// Listen on loopback only; report it as if it came from the LAN.
	var ln net.Listener
	m.listen = func(port int) (net.Listener, error) {
		mu.Lock()
		defer mu.Unlock()
		l, err := net.Listen("tcp", "127.0.0.1:0")
		ln = l
		return l, err
	}
	m.Configure(true, []int{2323}, nil)
	defer m.Close()
	if st := m.Status(); len(st.Ports) != 1 || st.Ports[0].State != StateOpen || st.Ports[0].Service != "telnet" {
		t.Fatalf("status = %+v", st)
	}
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, _ := c.Read(buf)
	if !strings.Contains(string(buf[:n]), "login:") {
		t.Fatalf("greeting = %q", buf[:n])
	}
	// Loopback is never reported.
	select {
	case h := <-hits:
		t.Fatalf("loopback reported: %+v", h)
	case <-time.After(100 * time.Millisecond):
	}

	m.Configure(false, nil, nil)
	if st := m.Status(); len(st.Ports) != 0 || st.Enabled {
		t.Fatalf("after disable: %+v", st)
	}
	if _, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second); err == nil {
		t.Fatal("decoy still open after disabling")
	}
}

func TestPrivilegedPorts(t *testing.T) {
	pre, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	m := New(quietLog(), map[int]net.Listener{23: pre}, nil, nil)
	m.listen = func(int) (net.Listener, error) { return nil, &net.OpError{Op: "listen", Err: errDenied} }
	m.Configure(true, []int{445}, nil)
	st := m.Status()
	if len(st.Ports) != 2 || st.Ports[0].Port != 23 || st.Ports[0].State != StateOpen || st.Ports[1].State != StateRestart {
		t.Fatalf("status = %+v", st.Ports)
	}
	m.Configure(false, nil, nil)
	m.Configure(true, nil, nil)
	if st := m.Status(); len(st.Ports) != 1 || st.Ports[0].State != StateRestart {
		t.Fatalf("after reopen: %+v", st.Ports)
	}
}

var errDenied = &net.AddrError{Err: "permission denied"}

func TestService(t *testing.T) {
	for port, want := range map[int]string{23: "telnet", 3389: "rdp", 8081: "camera", 5900: "vnc", 12345: "tcp"} {
		if got := Service(port); got != want {
			t.Errorf("Service(%d) = %q, want %q", port, got, want)
		}
	}
	if _, err := ParsePorts([]string{"23", "0"}); err == nil {
		t.Error("port 0 accepted")
	}
}
