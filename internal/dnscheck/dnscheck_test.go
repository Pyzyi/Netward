package dnscheck

import (
	"context"
	"net"
	"net/netip"
	"testing"

	"github.com/Pyzyi/netzwart/internal/dnscheck/dnschecktest"
)

var honest = dnschecktest.Honest

func dial(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}

func TestHonestServer(t *testing.T) {
	r := Check(context.Background(), dnschecktest.Server(t, honest, ""), dial)
	if r.Verdict != VerdictOK {
		t.Fatalf("verdict %s: %+v", r.Verdict, r)
	}
	if r.Missing.Status != StatusOK {
		t.Fatalf("missing name: %+v", r.Missing)
	}
}

func TestManipulatedServer(t *testing.T) {
	bad := map[string]string{"one.one.one.one": "1.1.1.1", "dns.google": "10.66.0.5", "dns.quad9.net": "9.9.9.9"}
	r := Check(context.Background(), dnschecktest.Server(t, bad, ""), dial)
	if r.Verdict != VerdictManipulated || r.Answers[1].Status != StatusWrong || r.Answers[1].Got[0] != "10.66.0.5" {
		t.Fatalf("got %+v", r)
	}
}

func TestBlockerIsNotManipulation(t *testing.T) {
	// Pi-hole style: dns.google sinkholed, quad9 "does not exist".
	blocked := map[string]string{"one.one.one.one": "1.1.1.1", "dns.google": "0.0.0.0"}
	r := Check(context.Background(), dnschecktest.Server(t, blocked, ""), dial)
	if r.Verdict != VerdictOK || r.Answers[1].Status != StatusBlocked || r.Answers[2].Status != StatusBlocked {
		t.Fatalf("got %+v", r)
	}
}

func TestRedirectingServer(t *testing.T) {
	r := Check(context.Background(), dnschecktest.Server(t, honest, "93.184.1.1"), dial)
	if r.Verdict != VerdictRedirects || r.Missing.Got[0] != "93.184.1.1" {
		t.Fatalf("got %+v", r)
	}
}

func TestUnreachableServer(t *testing.T) {
	// A closed UDP port: nothing answers.
	pc, _ := net.ListenPacket("udp4", "127.0.0.1:0")
	ap := netip.MustParseAddrPort(pc.LocalAddr().String())
	pc.Close()
	r := Check(context.Background(), ap, dial)
	if r.Verdict != VerdictUnreachable {
		t.Fatalf("got %+v", r)
	}
}
