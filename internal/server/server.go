// Package server is the daemon's localhost HTTP API: hook events in, session list out,
// plus the browser simulator (embedded design page fed via server-sent events).
package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/d135-1r43/cyberviber/design"
	"github.com/d135-1r43/cyberviber/internal/event"
	"github.com/d135-1r43/cyberviber/internal/protocol"
	"github.com/d135-1r43/cyberviber/internal/session"
)

type Server struct {
	store    *session.Store
	onChange func()

	// Focus brings the session's terminal to the front. nil → "not-implemented".
	Focus func(id string) (ok bool, reason string)

	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

// New creates the API. onChange (may be nil) is called after every accepted event.
func New(store *session.Store, onChange func()) *Server {
	return &Server{store: store, onChange: onChange, subs: map[chan []byte]struct{}{}}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /event", s.postEvent)
	mux.HandleFunc("GET /sessions", s.getSessions)
	mux.HandleFunc("GET /stream", s.stream)
	mux.HandleFunc("POST /focus", s.focus)
	mux.HandleFunc("GET /{$}", s.index)
	return localOnly(mux)
}

// Notify pushes the current session list to all stream subscribers.
func (s *Server) Notify() {
	msg := s.snapshot()
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subs {
		select { // keep only the latest message for slow subscribers
		case <-ch:
		default:
		}
		ch <- msg
	}
}

func (s *Server) snapshot() []byte {
	b, _ := json.Marshal(protocol.FromSessions(time.Now().Unix(), s.store.List()))
	return b
}

func (s *Server) postEvent(w http.ResponseWriter, r *http.Request) {
	var e event.Event
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&e); err != nil || e.SessionID == "" || e.Name == "" {
		http.Error(w, "bad event", http.StatusBadRequest)
		return
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	s.store.Apply(e)
	if s.onChange != nil {
		s.onChange()
	}
	s.Notify()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getSessions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(s.snapshot())
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	ch := make(chan []byte, 1)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}()

	fmt.Fprintf(w, "data: %s\n\n", s.snapshot())
	fl.Flush()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
		}
		fl.Flush()
	}
}

func (s *Server) focus(w http.ResponseWriter, r *http.Request) {
	var req protocol.Focus
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil || req.ID == "" {
		http.Error(w, "bad focus request", http.StatusBadRequest)
		return
	}
	res := protocol.Focused{Type: "focused", ID: req.ID, Reason: "not-implemented"}
	if s.Focus != nil {
		res.OK, res.Reason = s.Focus(req.ID)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (s *Server) index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(design.Index)
}

// localOnly blocks DNS rebinding (Host must be a loopback name) and requests made by
// other websites (an Origin header, if present, must match Host). Hook calls send no Origin.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" {
			u, err := url.Parse(o)
			if err != nil || u.Host != r.Host {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
