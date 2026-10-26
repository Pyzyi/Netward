package proto

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"golang.org/x/net/dns/dnsmessage"
)

// MDNSInstance is a DNS-SD service instance ("Wohnzimmer._airplay._tcp").
type MDNSInstance struct {
	Service  string // "_airplay._tcp"
	Instance string // "Wohnzimmer"
	Port     uint16
}

// MDNSInfo is what a single mDNS packet reveals about its sender.
type MDNSInfo struct {
	Hostname     string // from an A record pointing at the sender, or a reverse PTR
	Instances    []MDNSInstance
	ServiceTypes []string    // from _services._dns-sd._udp browsing
	Model        string      // model hints from TXT records (model=, md=, am=, ty=)
	FriendlyName string      // fn= (Google Cast)
	HomeKitCat   string      // ci= (HomeKit accessory category)
	TXT          []TXTRecord // version-related TXT keys (see VersionTXT)
	Response     bool
}

// TXTRecord holds the whitelisted keys of one service's TXT record.
type TXTRecord struct {
	Service string
	KV      map[string]string
}

// VersionTXT lists, per DNS-SD service type, the TXT keys that carry version
// information. Nothing else from these records is kept: no serial numbers,
// no device IDs (T-33).
var VersionTXT = map[string][]string{
	"_airplay._tcp":        {"osvers", "model"},
	"_device-info._tcp":    {"osxvers", "model"},
	"_http._tcp":           {"vendor", "version_major", "version_minor", "version_build"},
	"_esphomelib._tcp":     {"version"},
	"_shelly._tcp":         {"ver", "gen"},
	"_home-assistant._tcp": {"version"},
}

const maxTXTRecords = 8

const maxMDNSRecords = 96

// ReverseName returns the in-addr.arpa name for an IPv4 address.
func ReverseName(ip netip.Addr) string {
	b := ip.As4()
	return fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa.", b[3], b[2], b[1], b[0])
}

// BuildMDNSQuery builds a query for names of type t. With unicastResponse the
// QU bit asks responders to answer directly instead of via multicast.
func BuildMDNSQuery(id uint16, t dnsmessage.Type, unicastResponse bool, names ...string) ([]byte, error) {
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: id})
	b.EnableCompression()
	if err := b.StartQuestions(); err != nil {
		return nil, err
	}
	class := dnsmessage.ClassINET
	if unicastResponse {
		class |= 1 << 15
	}
	for _, n := range names {
		name, err := dnsmessage.NewName(n)
		if err != nil {
			return nil, err
		}
		if err := b.Question(dnsmessage.Question{Name: name, Type: t, Class: class}); err != nil {
			return nil, err
		}
	}
	return b.Finish()
}

func trimLocal(n string) string {
	n = strings.TrimSuffix(n, ".")
	return strings.TrimSuffix(n, ".local")
}

// serviceOf splits "<instance>.<_svc>.<_proto>.local." into instance and service.
func serviceOf(full string) (instance, service string, ok bool) {
	full = strings.TrimSuffix(full, ".")
	full = strings.TrimSuffix(full, ".local")
	i := strings.LastIndex(full, "._tcp")
	if i < 0 {
		i = strings.LastIndex(full, "._udp")
	}
	if i < 0 || i+5 != len(full) {
		return "", "", false
	}
	head := full[:i] // "<instance>._svc"
	j := strings.LastIndex(head, "._")
	if j < 0 {
		if strings.HasPrefix(head, "_") {
			return "", full, true
		}
		return "", "", false
	}
	return head[:j], full[j+1:], true
}

// ParseMDNS extracts information about src from an mDNS message.
func ParseMDNS(p []byte, src netip.Addr) (MDNSInfo, error) {
	var info MDNSInfo
	var parser dnsmessage.Parser
	h, err := parser.Start(p)
	if err != nil {
		return info, err
	}
	info.Response = h.Response
	if err := parser.SkipAllQuestions(); err != nil {
		return info, err
	}
	reverse := ReverseName(src)
	instances := map[string]*MDNSInstance{}
	seenSvc := map[string]bool{}
	count := 0
	for section := 0; section < 3; section++ {
		next, skip := parser.AnswerHeader, parser.SkipAnswer
		switch section {
		case 1:
			next, skip = parser.AuthorityHeader, parser.SkipAuthority
		case 2:
			next, skip = parser.AdditionalHeader, parser.SkipAdditional
		}
	records:
		for {
			hdr, err := next()
			if err == dnsmessage.ErrSectionDone {
				break records
			}
			if err != nil {
				return info, err
			}
			count++
			if count > maxMDNSRecords {
				return info, nil
			}
			owner := hdr.Name.String()
			switch hdr.Type {
			case dnsmessage.TypeA:
				r, err := parser.AResource()
				if err != nil {
					return info, err
				}
				if src.Is4() && netip.AddrFrom4(r.A) == src && strings.HasSuffix(strings.ToLower(owner), ".local.") {
					info.Hostname = trimLocal(owner)
				}
			case dnsmessage.TypePTR:
				r, err := parser.PTRResource()
				if err != nil {
					return info, err
				}
				target := r.PTR.String()
				switch {
				case strings.EqualFold(owner, reverse):
					info.Hostname = trimLocal(target)
				case strings.EqualFold(owner, "_services._dns-sd._udp.local."):
					if _, svc, ok := serviceOf(target); ok && !seenSvc[svc] {
						seenSvc[svc] = true
						info.ServiceTypes = append(info.ServiceTypes, svc)
					}
				default:
					if inst, svc, ok := serviceOf(target); ok && inst != "" {
						if _, dup := instances[target]; !dup {
							instances[target] = &MDNSInstance{Service: svc, Instance: inst}
						}
					}
				}
			case dnsmessage.TypeSRV:
				r, err := parser.SRVResource()
				if err != nil {
					return info, err
				}
				if inst, svc, ok := serviceOf(owner); ok && inst != "" {
					it := instances[owner]
					if it == nil {
						it = &MDNSInstance{Service: svc, Instance: inst}
						instances[owner] = it
					}
					it.Port = r.Port
				}
			case dnsmessage.TypeTXT:
				r, err := parser.TXTResource()
				if err != nil {
					return info, err
				}
				if _, svc, ok := serviceOf(owner); ok && len(info.TXT) < maxTXTRecords {
					if keys := VersionTXT[svc]; keys != nil {
						kv := map[string]string{}
						for _, s := range r.TXT {
							k, v, ok := strings.Cut(s, "=")
							k = strings.ToLower(k)
							if ok && v != "" && len(v) <= 64 && slices.Contains(keys, k) {
								kv[k] = v
							}
						}
						if len(kv) > 0 {
							info.TXT = append(info.TXT, TXTRecord{Service: svc, KV: kv})
						}
					}
				}
				for _, kv := range r.TXT {
					k, v, ok := strings.Cut(kv, "=")
					if !ok || v == "" {
						continue
					}
					switch strings.ToLower(k) {
					case "model", "md", "am", "ty", "usb_mdl", "product":
						if info.Model == "" {
							info.Model = strings.Trim(v, "()")
						}
					case "fn":
						if info.FriendlyName == "" {
							info.FriendlyName = v
						}
					case "ci":
						if strings.Contains(strings.ToLower(owner), "._hap._tcp") {
							info.HomeKitCat = v
						}
					}
				}
			default:
				if err := skip(); err != nil {
					return info, err
				}
			}
		}
	}
	for _, it := range instances {
		info.Instances = append(info.Instances, *it)
		if !seenSvc[it.Service] {
			seenSvc[it.Service] = true
			info.ServiceTypes = append(info.ServiceTypes, it.Service)
		}
	}
	return info, nil
}
