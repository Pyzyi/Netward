package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const maxStreams = 32

// streamRegistry lets logout and session revocation end open streams immediately.
type streamRegistry struct {
	mu      sync.Mutex
	streams map[int64]map[*context.CancelFunc]struct{}
	n       int
}

func newStreamRegistry() *streamRegistry {
	return &streamRegistry{streams: map[int64]map[*context.CancelFunc]struct{}{}}
}

func (s *streamRegistry) add(session int64, cancel *context.CancelFunc) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.n >= maxStreams {
		return false
	}
	if s.streams[session] == nil {
		s.streams[session] = map[*context.CancelFunc]struct{}{}
	}
	s.streams[session][cancel] = struct{}{}
	s.n++
	return true
}

func (s *streamRegistry) remove(session int64, cancel *context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.streams[session][cancel]; ok {
		delete(s.streams[session], cancel)
		s.n--
	}
	if len(s.streams[session]) == 0 {
		delete(s.streams, session)
	}
}

func (s *streamRegistry) closeSession(session int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.streams[session] {
		(*c)()
	}
}

func (s *streamRegistry) closeAllExcept(session int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, set := range s.streams {
		if id == session {
			continue
		}
		for c := range set {
			(*c)()
		}
	}
}

func (s *streamRegistry) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

// handleStream sends live updates as Server-Sent Events.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request, si *sessionInfo) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	if !s.streams.add(si.session.ID, &cancel) {
		writeError(w, http.StatusServiceUnavailable, "too_many_streams", "")
		return
	}
	defer s.streams.remove(si.session.ID, &cancel)

	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{}) // long-lived response
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	sub := s.d.Hub.Subscribe(512)
	defer s.d.Hub.Unsubscribe(sub)
	send := func(event string, v any) bool {
		data, err := json.Marshal(v)
		if err != nil {
			return true
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !send("hello", map[string]any{"version": s.d.Version}) {
		return
	}
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			// Re-validate the session so expiry ends the stream too.
			if _, err := s.d.Store.GetSessionByHash(ctx, si.session.TokenHash, s.now().UnixMilli()); err != nil {
				return
			}
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		case m := <-sub.C:
			if sub.Dropped() {
				if !send("resync", struct{}{}) {
					return
				}
				continue
			}
			if !send(m.Type, m.Data) {
				return
			}
		}
	}
}
