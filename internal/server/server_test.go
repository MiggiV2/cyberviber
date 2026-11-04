package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/d135-1r43/cyberviber/internal/event"
	"github.com/d135-1r43/cyberviber/internal/protocol"
	"github.com/d135-1r43/cyberviber/internal/session"
)

func TestShouldAcceptEventsAndServeSessions(t *testing.T) {
	// given
	store := session.New(10*time.Minute, func(string) string { return "main" })
	changed := 0
	srv := httptest.NewServer(New(store, func() { changed++ }).Handler())
	defer srv.Close()
	e := event.Event{Name: event.PermissionRequest, SessionID: "a1", Cwd: "/x/quarkus-svc", Activity: "Allow Bash: ls?",
		Time: time.Now(), ZellijSession: "work", ZellijPane: "3"}
	body, _ := json.Marshal(e)

	// when
	resp, err := http.Post(srv.URL+"/event", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// then
	if resp.StatusCode != http.StatusNoContent || changed != 1 {
		t.Fatalf("status %d changed %d", resp.StatusCode, changed)
	}
	r, err := http.Get(srv.URL + "/sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var msg protocol.Sessions
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "sessions" || len(msg.Sessions) != 1 {
		t.Fatalf("unexpected %+v", msg)
	}
	got := msg.Sessions[0]
	if got.ID != "a1" || got.Name != "quarkus-svc" || got.State != "input" || got.Zellij != "work" || got.Activity != "Allow Bash: ls?" {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestShouldRejectBadRequests(t *testing.T) {
	store := session.New(time.Minute, func(string) string { return "" })
	srv := httptest.NewServer(New(store, nil).Handler())
	defer srv.Close()

	r, _ := http.Post(srv.URL+"/event", "application/json", bytes.NewReader([]byte("nope")))
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d", r.StatusCode)
	}
	r, _ = http.Post(srv.URL+"/event", "application/json", bytes.NewReader([]byte(`{"hook_event_name":"Stop"}`)))
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing session id: got %d", r.StatusCode)
	}
	r, _ = http.Get(srv.URL + "/event")
	if r.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("got %d", r.StatusCode)
	}
}
