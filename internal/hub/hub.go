// Package hub fans out live updates (device changes, events, scan progress)
// to subscribers such as SSE clients and the MQTT bridge.
package hub

import "sync"

// Message is one update.
type Message struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Hub is a non-blocking publish/subscribe hub. Slow subscribers lose
// messages and are told to resynchronise.
type Hub struct {
	mu   sync.Mutex
	subs map[*Sub]struct{}
}

// Sub is a subscription.
type Sub struct {
	C       chan Message
	mu      sync.Mutex
	dropped bool
}

// Dropped reports (and clears) whether messages were lost since the last call.
func (s *Sub) Dropped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.dropped
	s.dropped = false
	return d
}

// New returns an empty hub.
func New() *Hub { return &Hub{subs: map[*Sub]struct{}{}} }

// Subscribe registers a subscriber with the given buffer size.
func (h *Hub) Subscribe(buffer int) *Sub {
	s := &Sub{C: make(chan Message, buffer)}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()
	return s
}

// Unsubscribe removes s. Its channel is not closed, so pending readers just stop.
func (h *Hub) Unsubscribe(s *Sub) {
	h.mu.Lock()
	delete(h.subs, s)
	h.mu.Unlock()
}

// Count returns the number of subscribers.
func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// Publish delivers m to every subscriber without blocking.
func (h *Hub) Publish(typ string, data any) {
	if h == nil {
		return
	}
	m := Message{Type: typ, Data: data}
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		select {
		case s.C <- m:
		default:
			s.mu.Lock()
			s.dropped = true
			s.mu.Unlock()
		}
	}
}
