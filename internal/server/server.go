// Package server is the daemon's localhost HTTP API: hook events in, session list out.
package server

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"cyberviber/internal/event"
	"cyberviber/internal/protocol"
	"cyberviber/internal/session"
)

type Server struct {
	store    *session.Store
	onChange func()
}

// New creates the API. onChange (may be nil) is called after every accepted event.
func New(store *session.Store, onChange func()) *Server {
	return &Server{store: store, onChange: onChange}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /event", s.postEvent)
	mux.HandleFunc("GET /sessions", s.getSessions)
	return mux
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
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getSessions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(protocol.FromSessions(time.Now().Unix(), s.store.List()))
}
