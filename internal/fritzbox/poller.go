package fritzbox

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Pyzyi/netzwart/internal/config"
	"github.com/Pyzyi/netzwart/internal/hub"
	"github.com/Pyzyi/netzwart/internal/store"
)

const (
	pollEvery  = time.Minute
	retryAfter = 5 * time.Minute
	pinKey     = "fritzbox_pin"
)

// Status is shown in the settings page.
type Status struct {
	State    string `json:"state"`           // disabled | connecting | connected | error
	Error    string `json:"error,omitempty"` // auth | cert | unreachable | protocol
	Hosts    int    `json:"hosts"`
	LastSync int64  `json:"lastSync"`
}

// ApplyFunc receives the host table after every successful sync.
type ApplyFunc func(ctx context.Context, hosts []Host, at time.Time)

type pin struct {
	Host   string `json:"host"`
	SHA256 string `json:"sha256"`
}

// Poller syncs the host table periodically while the integration is enabled.
type Poller struct {
	st    *store.Store
	hub   *hub.Hub
	dial  DialFunc
	apply ApplyFunc
	log   *slog.Logger

	mu       sync.Mutex
	cfg      config.FritzBox
	status   Status
	trigger  chan struct{}
	firmware func(ctx context.Context, host string, fw Firmware)
	fwAt     time.Time
}

// firmwareEvery: FRITZ!OS updates are rare; asking every few hours is plenty.
const firmwareEvery = 6 * time.Hour

// OnFirmware registers fn to receive the router's firmware state.
func (p *Poller) OnFirmware(fn func(ctx context.Context, host string, fw Firmware)) {
	p.mu.Lock()
	p.firmware = fn
	p.mu.Unlock()
}

// NewPoller creates a disabled poller.
func NewPoller(st *store.Store, h *hub.Hub, dial DialFunc, apply ApplyFunc, log *slog.Logger) *Poller {
	return &Poller{st: st, hub: h, dial: dial, apply: apply, log: log,
		status: Status{State: "disabled"}, trigger: make(chan struct{}, 1)}
}

// Configure applies new settings. Switching the integration off forgets the
// pinned certificate, which is how a replaced router is trusted again.
func (p *Poller) Configure(ctx context.Context, cfg config.FritzBox) {
	p.mu.Lock()
	changed := cfg != p.cfg
	p.cfg = cfg
	if changed {
		p.fwAt = time.Time{}
	}
	if !cfg.Enabled {
		p.status = Status{State: "disabled"}
	} else if changed {
		p.status = Status{State: "connecting"}
	}
	p.mu.Unlock()
	if !cfg.Enabled {
		_ = p.st.SetSetting(ctx, pinKey, pin{})
	}
	if changed {
		select {
		case p.trigger <- struct{}{}:
		default:
		}
	}
}

// Status returns the current state.
func (p *Poller) Status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}

// Run syncs until ctx ends.
func (p *Poller) Run(ctx context.Context) {
	t := time.NewTimer(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-p.trigger:
		}
		wait := pollEvery
		if err := p.Sync(ctx); err != nil {
			wait = retryAfter
		}
		if !t.Stop() {
			select {
			case <-t.C:
			default:
			}
		}
		t.Reset(wait)
	}
}

// Sync reads the host table once. It does nothing while disabled.
func (p *Poller) Sync(ctx context.Context) error {
	p.mu.Lock()
	cfg := p.cfg
	p.mu.Unlock()
	if !cfg.Enabled || cfg.Host == "" {
		return nil
	}
	var stored pin
	_ = p.st.GetSetting(ctx, pinKey, &stored)
	var known []byte
	if stored.Host == cfg.Host {
		known, _ = hex.DecodeString(stored.SHA256)
	}
	if len(known) == 0 {
		known = nil
	}
	c := NewClient(cfg.Host, cfg.Username, string(cfg.Password), known, p.dial)
	defer c.Close()
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	hosts, err := c.Hosts(cctx)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		code := ErrProtocol.Error()
		for _, e := range []error{ErrAuth, ErrCertChanged, ErrUnreachable} {
			if errors.Is(err, e) {
				code = e.Error()
			}
		}
		p.setStatus(ctx, cfg, Status{State: "error", Error: code})
		p.log.Warn("FRITZ!Box sync failed", "host", cfg.Host, "err", err)
		return err
	}
	if known == nil {
		if sum := c.Pin(); sum != nil {
			_ = p.st.SetSetting(ctx, pinKey, pin{Host: cfg.Host, SHA256: hex.EncodeToString(sum)})
			p.log.Info("FRITZ!Box certificate pinned", "host", cfg.Host, "sha256", hex.EncodeToString(sum))
		}
	}
	now := time.Now()
	p.apply(ctx, hosts, now)
	p.mu.Lock()
	fwFn, fwDue := p.firmware, now.Sub(p.fwAt) >= firmwareEvery
	if fwDue {
		p.fwAt = now
	}
	p.mu.Unlock()
	if fwFn != nil && fwDue {
		if fw, err := c.Firmware(cctx); err == nil && fw.Version != "" {
			fwFn(ctx, cfg.Host, fw)
		} else if err != nil {
			p.log.Info("FRITZ!Box firmware query failed", "err", err)
		}
	}
	p.setStatus(ctx, cfg, Status{State: "connected", Hosts: len(hosts), LastSync: now.UnixMilli()})
	return nil
}

// setStatus stores st unless the settings changed during the sync.
func (p *Poller) setStatus(_ context.Context, cfg config.FritzBox, st Status) {
	p.mu.Lock()
	if p.cfg != cfg {
		p.mu.Unlock()
		return
	}
	p.status = st
	p.mu.Unlock()
	p.hub.Publish("fritzbox", st)
}
