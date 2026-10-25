package proto

import (
	"net/netip"
	"slices"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func mdnsResponse(t testing.TB) []byte {
	t.Helper()
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true, Authoritative: true})
	b.EnableCompression()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	name := func(s string) dnsmessage.Name { return dnsmessage.MustNewName(s) }
	rh := func(n string, typ dnsmessage.Type) dnsmessage.ResourceHeader {
		return dnsmessage.ResourceHeader{Name: name(n), Type: typ, Class: dnsmessage.ClassINET, TTL: 120}
	}
	must(b.StartAnswers())
	must(b.PTRResource(rh("_airplay._tcp.local.", dnsmessage.TypePTR), dnsmessage.PTRResource{PTR: name("Wohnzimmer._airplay._tcp.local.")}))
	must(b.PTRResource(rh("_services._dns-sd._udp.local.", dnsmessage.TypePTR), dnsmessage.PTRResource{PTR: name("_raop._tcp.local.")}))
	must(b.StartAdditionals())
	must(b.SRVResource(rh("Wohnzimmer._airplay._tcp.local.", dnsmessage.TypeSRV), dnsmessage.SRVResource{Port: 7000, Target: name("Apple-TV.local.")}))
	must(b.TXTResource(rh("Wohnzimmer._airplay._tcp.local.", dnsmessage.TypeTXT), dnsmessage.TXTResource{TXT: []string{"model=AppleTV11,1", "flags=0x4"}}))
	must(b.AResource(rh("Apple-TV.local.", dnsmessage.TypeA), dnsmessage.AResource{A: [4]byte{192, 168, 1, 40}}))
	must(b.AResource(rh("Other.local.", dnsmessage.TypeA), dnsmessage.AResource{A: [4]byte{192, 168, 1, 99}}))
	msg, err := b.Finish()
	must(err)
	return msg
}

func TestParseMDNS(t *testing.T) {
	info, err := ParseMDNS(mdnsResponse(t), netip.MustParseAddr("192.168.1.40"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.Response || info.Hostname != "Apple-TV" || info.Model != "AppleTV11,1" {
		t.Fatalf("unexpected info %+v", info)
	}
	if len(info.Instances) != 1 || info.Instances[0].Instance != "Wohnzimmer" || info.Instances[0].Service != "_airplay._tcp" || info.Instances[0].Port != 7000 {
		t.Fatalf("unexpected instances %+v", info.Instances)
	}
	if !slices.Contains(info.ServiceTypes, "_raop._tcp") || !slices.Contains(info.ServiceTypes, "_airplay._tcp") {
		t.Fatalf("unexpected service types %v", info.ServiceTypes)
	}
	// A records for other addresses must not name the sender (sleep proxies).
	other, _ := ParseMDNS(mdnsResponse(t), netip.MustParseAddr("192.168.1.50"))
	if other.Hostname != "" {
		t.Fatalf("foreign A record used as hostname: %q", other.Hostname)
	}
}

func TestReverseQuery(t *testing.T) {
	ip := netip.MustParseAddr("192.168.1.40")
	if ReverseName(ip) != "40.1.168.192.in-addr.arpa." {
		t.Fatal(ReverseName(ip))
	}
	q, err := BuildMDNSQuery(0, dnsmessage.TypePTR, true, ReverseName(ip))
	if err != nil {
		t.Fatal(err)
	}
	// Answer it the way an Apple device does.
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true})
	_ = b.StartAnswers()
	_ = b.PTRResource(dnsmessage.ResourceHeader{Name: dnsmessage.MustNewName(ReverseName(ip)), Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET},
		dnsmessage.PTRResource{PTR: dnsmessage.MustNewName("Annas-MacBook.local.")})
	resp, _ := b.Finish()
	info, err := ParseMDNS(resp, ip)
	if err != nil || info.Hostname != "Annas-MacBook" || len(q) < 12 {
		t.Fatalf("reverse lookup parse failed: %+v %v", info, err)
	}
}

func TestServiceOf(t *testing.T) {
	cases := map[string][2]string{
		"Living Room._googlecast._tcp.local.": {"Living Room", "_googlecast._tcp"},
		"_hap._tcp.local.":                    {"", "_hap._tcp"},
		"a.b.c._ipp._tcp.local":               {"a.b.c", "_ipp._tcp"},
	}
	for in, want := range cases {
		inst, svc, ok := serviceOf(in)
		if !ok || inst != want[0] || svc != want[1] {
			t.Errorf("serviceOf(%q) = %q, %q, %v", in, inst, svc, ok)
		}
	}
	if _, _, ok := serviceOf("printer.local."); ok {
		t.Error("plain host accepted as service")
	}
}

func FuzzParseMDNS(f *testing.F) {
	f.Add(mdnsResponse(f))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = ParseMDNS(b, netip.MustParseAddr("192.168.1.40"))
	})
}
