package notify

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/Pyzyi/netzwart/internal/egress"
	"github.com/Pyzyi/netzwart/internal/store"
)

// batchWindow collects new-device events so a burst becomes one message.
const batchWindow = 20 * time.Second

// Dispatcher routes events to the configured channels with batching,
// retries and a per-channel rate limit.
type Dispatcher struct {
	st     *store.Store
	log    *slog.Logger
	policy *egress.Policy
	client *http.Client
	queue  chan Event

	mu       sync.Mutex
	lang     string
	loc      *time.Location
	channels []store.Channel
	sent     map[int64][]time.Time // channel -> recent send times
}

// NewDispatcher creates a dispatcher; call Reload after channel changes.
func NewDispatcher(st *store.Store, policy *egress.Policy, log *slog.Logger) *Dispatcher {
	return &Dispatcher{st: st, log: log, policy: policy, client: policy.HTTPClient(10*time.Second, 0), // no redirects: secrets in headers stay with their destination
		queue: make(chan Event, 512), lang: "de", loc: time.UTC, sent: map[int64][]time.Time{}}
}

// SetLocale sets language and time zone for messages.
func (d *Dispatcher) SetLocale(lang, tz string) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	d.mu.Lock()
	d.lang, d.loc = lang, loc
	d.mu.Unlock()
}

// Reload reads the channel list from the database.
func (d *Dispatcher) Reload(ctx context.Context) error {
	chs, err := d.st.ListChannels(ctx)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.channels = chs
	d.mu.Unlock()
	return nil
}

// Notify implements Notifier. It never blocks.
func (d *Dispatcher) Notify(ev Event) {
	if ev.TS == 0 {
		ev.TS = time.Now().UnixMilli()
	}
	select {
	case d.queue <- ev:
	default:
		d.log.Warn("notification queue full; dropping event", "kind", ev.Kind)
	}
}

// Run processes the queue until ctx ends.
func (d *Dispatcher) Run(ctx context.Context) {
	var batch []Event
	var flush <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-d.queue:
			if ev.Kind == KindDeviceNew {
				batch = append(batch, ev)
				if flush == nil {
					flush = time.After(batchWindow)
				}
				continue
			}
			d.deliver(ctx, ev.Kind, d.render(ev))
		case <-flush:
			flush = nil
			switch {
			case len(batch) == 0:
			case len(batch) <= 3:
				for _, ev := range batch {
					d.deliver(ctx, ev.Kind, d.render(ev))
				}
			default:
				d.mu.Lock()
				lang := d.lang
				d.mu.Unlock()
				d.deliver(ctx, KindDeviceNew, RenderSummary(batch, lang))
			}
			batch = nil
		}
	}
}

func (d *Dispatcher) render(ev Event) Message {
	d.mu.Lock()
	lang, loc := d.lang, d.loc
	d.mu.Unlock()
	return Render(ev, lang, loc)
}

// allow implements a per-channel limit of 30 messages per 10 minutes.
func (d *Dispatcher) allow(id int64, now time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	recent := d.sent[id][:0]
	for _, t := range d.sent[id] {
		if now.Sub(t) < 10*time.Minute {
			recent = append(recent, t)
		}
	}
	if len(recent) >= 30 {
		d.sent[id] = recent
		return false
	}
	d.sent[id] = append(recent, now)
	return true
}

func (d *Dispatcher) deliver(ctx context.Context, kind string, msg Message) {
	d.mu.Lock()
	chs := slices.Clone(d.channels)
	d.mu.Unlock()
	for _, ch := range chs {
		if !ch.Enabled || !slices.Contains(ch.Events, kind) {
			continue
		}
		if !d.allow(ch.ID, time.Now()) {
			d.log.Warn("notification rate limit reached", "channel", ch.Name)
			continue
		}
		go d.sendWithRetry(ctx, ch, msg)
	}
}

func (d *Dispatcher) sendWithRetry(ctx context.Context, ch store.Channel, msg Message) {
	delays := []time.Duration{0, 15 * time.Second, 2 * time.Minute}
	for i, delay := range delays {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}
		err := Send(ctx, d.client, d.policy, ch.Kind, ch.Config, msg)
		if err == nil {
			return
		}
		d.log.Warn("notification failed", "channel", ch.Name, "kind", ch.Kind, "attempt", i+1, "reason", err.Error())
	}
}

// Test sends a test message synchronously and returns a short reason on failure.
func (d *Dispatcher) Test(ctx context.Context, ch store.Channel) error {
	d.mu.Lock()
	lang := d.lang
	d.mu.Unlock()
	msg := Render(Event{Kind: KindTest, Severity: "info", TS: time.Now().UnixMilli()}, lang, time.UTC)
	return Send(ctx, d.client, d.policy, ch.Kind, ch.Config, msg)
}
