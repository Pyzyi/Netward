package fritzbox

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

// Line is the state of the internet connection as the router sees it
// (docs/09, F5). Sync rates are what the modem negotiated with the
// provider, not the throughput a speed test measures.
type Line struct {
	Access      string // dsl, cable, fiber, ethernet, mobile or ""
	Up          bool   // physical link up
	DownKbps    int    // current sync / link rate
	UpKbps      int
	DownMaxKbps int // attainable rate (DSL only)
	UpMaxKbps   int
	Uptime      int64 // seconds since the internet connection was established; 0 = unknown
}

func accessType(s string) string {
	switch t := strings.ToLower(strings.TrimPrefix(s, "X_AVM-DE_")); t {
	case "dsl", "cable", "fiber", "ethernet":
		return t
	case "umts", "lte", "5g", "mobile":
		return "mobile"
	}
	return ""
}

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// Line reads the connection state: link properties for every access type,
// the DSL details (sync and attainable rates) where there is DSL, and the
// connection uptime to tell reconnects.
func (c *Client) Line(ctx context.Context) (Line, error) {
	base, err := c.secureBase(ctx)
	if err != nil {
		return Line{}, err
	}
	common, err := c.soap(ctx, base, "/upnp/control/wancommonifconfig1", "urn:dslforum-org:service:WANCommonInterfaceConfig:1", "GetCommonLinkProperties", nil)
	if err != nil {
		return Line{}, err
	}
	l := Line{
		Access:   accessType(common["NewWANAccessType"]),
		Up:       strings.EqualFold(common["NewPhysicalLinkStatus"], "Up"),
		DownKbps: atoi(common["NewLayer1DownstreamMaxBitRate"]) / 1000,
		UpKbps:   atoi(common["NewLayer1UpstreamMaxBitRate"]) / 1000,
	}
	if l.Access == "dsl" {
		if d, err := c.soap(ctx, base, "/upnp/control/wandslifconfig1", "urn:dslforum-org:service:WANDSLInterfaceConfig:1", "GetInfo", nil); err == nil {
			if v := atoi(d["NewDownstreamCurrRate"]); v > 0 {
				l.DownKbps, l.UpKbps = v, atoi(d["NewUpstreamCurrRate"])
			}
			l.DownMaxKbps, l.UpMaxKbps = atoi(d["NewDownstreamMaxRate"]), atoi(d["NewUpstreamMaxRate"])
		} else if ctx.Err() != nil {
			return Line{}, err
		}
	}
	for _, svc := range [][2]string{
		{"/upnp/control/wanipconnection1", "urn:dslforum-org:service:WANIPConnection:1"},
		{"/upnp/control/wanpppconn1", "urn:dslforum-org:service:WANPPPConnection:1"},
	} {
		st, err := c.soap(ctx, base, svc[0], svc[1], "GetStatusInfo", nil)
		if err != nil {
			var fault *soapFault
			if errors.As(err, &fault) || errors.Is(err, ErrAuth) {
				continue
			}
			return l, nil // the rates are known; the uptime is a bonus
		}
		if strings.EqualFold(st["NewConnectionStatus"], "Connected") {
			if up := atoi(st["NewUptime"]); up > 0 {
				l.Uptime = int64(up)
				break
			}
		}
	}
	return l, nil
}
