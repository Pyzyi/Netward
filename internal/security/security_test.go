package security

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"github.com/Pyzyi/netzwart/internal/config"
	"github.com/Pyzyi/netzwart/internal/hub"
	"github.com/Pyzyi/netzwart/internal/l2"
	"github.com/Pyzyi/netzwart/internal/monitor"
	"github.com/Pyzyi/netzwart/internal/notify"
	"github.com/Pyzyi/netzwart/internal/store"
)

type rec struct{ evs []notify.Event }

func (r *rec) Notify(e notify.Event) { r.evs = append(r.evs, e) }

func TestDescribe(t *testing.T) {
	title, detail := Describe(CheckRiskyPort, map[string]any{"service": "telnet"}, "de")
	if title != "Riskanter Dienst erreichbar: TELNET" || detail == "" {
		t.Fatalf("%q %q", title, detail)
	}
	if title, _ := Describe(CheckUPnP, nil, "en"); title != "UPnP enabled on the router" {
		t.Fatal(title)
	}
}

func TestRunFindsUnknownDevicesAndPrunes(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := hub.New()
	mon, err := monitor.New(ctx, st, h, nil, config.Defaults(), log)
	if err != nil {
		t.Fatal(err)
	}
	// TEST-NET addresses never answer, so no risky ports are found.
	for i := byte(1); i <= 6; i++ {
		mon.Observe(ctx, l2.Sighting{MAC: net.HardwareAddr{0x3c, 0, 0, 0, 0, i}, IP: netip.AddrFrom4([4]byte{192, 0, 2, i}),
			Iface: "eth0", Source: "arp", ARP: true, At: time.Now()})
	}
	r := &rec{}
	c := New(st, mon, h, r, log)
	c.run(ctx)
	fs, _ := st.ListFindings(ctx)
	if len(fs) != 1 || fs[0].CheckID != CheckUnknown || fs[0].Severity != "warning" {
		t.Fatalf("unexpected findings %+v", fs)
	}
	if len(r.evs) != 0 {
		t.Fatal("unknown devices must not trigger a notification")
	}
	if c.Status().LastRunAt == 0 {
		t.Fatal("last run not recorded")
	}
	// After acknowledging everything the finding disappears on the next run.
	if _, err := mon.AcknowledgeAll(ctx); err != nil {
		t.Fatal(err)
	}
	c.run(ctx)
	fs, _ = st.ListFindings(ctx)
	if len(fs) != 0 {
		t.Fatalf("stale finding not pruned: %+v", fs)
	}
}
