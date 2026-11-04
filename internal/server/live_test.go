package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/d135-1r43/cyberviber/internal/event"
	"github.com/d135-1r43/cyberviber/internal/protocol"
	"github.com/d135-1r43/cyberviber/internal/session"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	store := session.New(10*time.Minute, func(string) string { return "" })
	api := New(store, nil)
	ts := httptest.NewServer(api.Handler())
	t.Cleanup(ts.Close)
	return api, ts
}

func postEvent(t *testing.T, url string, e event.Event) {
	t.Helper()
	b, _ := json.Marshal(e)
	r, err := http.Post(url+"/event", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
}

func readSSE(t *testing.T, br *bufio.Reader) protocol.Sessions {
	t.Helper()
	type res struct {
		msg protocol.Sessions
		err error
	}
	ch := make(chan res, 1)
	go func() {
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				ch <- res{err: err}
				return
			}
			if data, ok := strings.CutPrefix(strings.TrimSpace(line), "data: "); ok {
				var m protocol.Sessions
				ch <- res{msg: m, err: json.Unmarshal([]byte(data), &m)}
				return
			}
		}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatal(r.err)
		}
		return r.msg
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for SSE message")
	}
	return protocol.Sessions{}
}

func TestShouldServeSimulatorPage(t *testing.T) {
	_, ts := newTestServer(t)

	r, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || !strings.HasPrefix(r.Header.Get("Content-Type"), "text/html") || !strings.Contains(string(b), "cyberviber") {
		t.Fatalf("status %d type %q", r.StatusCode, r.Header.Get("Content-Type"))
	}
}

func TestShouldStreamSessionsOnConnectAndOnChange(t *testing.T) {
	// given
	_, ts := newTestServer(t)
	r, err := http.Get(ts.URL + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if ct := r.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	br := bufio.NewReader(r.Body)

	// then: initial snapshot
	if first := readSSE(t, br); first.Type != "sessions" || len(first.Sessions) != 0 {
		t.Fatalf("unexpected first message %+v", first)
	}

	// when
	postEvent(t, ts.URL, event.Event{Name: event.Stop, SessionID: "s1", Cwd: "/x/dotfiles", Time: time.Now()})

	// then
	if next := readSSE(t, br); len(next.Sessions) != 1 || next.Sessions[0].Name != "dotfiles" {
		t.Fatalf("unexpected update %+v", next)
	}
}

func TestShouldBroadcastOnNotify(t *testing.T) {
	// given
	api, ts := newTestServer(t)
	r, err := http.Get(ts.URL + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	br := bufio.NewReader(r.Body)
	readSSE(t, br)

	// when (e.g. after an idle transition in the daemon's tick)
	api.Notify()

	// then
	readSSE(t, br)
}

func TestShouldAnswerFocusWithoutFocuser(t *testing.T) {
	_, ts := newTestServer(t)

	r, err := http.Post(ts.URL+"/focus", "application/json", strings.NewReader(`{"id":"s1"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var got protocol.Focused
	json.NewDecoder(r.Body).Decode(&got)
	if got.Type != "focused" || got.ID != "s1" || got.OK || got.Reason != "not-implemented" {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestShouldUseFocuser(t *testing.T) {
	api, ts := newTestServer(t)
	var asked string
	api.Focus = func(id string) (bool, string) { asked = id; return true, "" }

	r, err := http.Post(ts.URL+"/focus", "application/json", strings.NewReader(`{"id":"s9"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var got protocol.Focused
	json.NewDecoder(r.Body).Decode(&got)
	if asked != "s9" || !got.OK {
		t.Fatalf("asked %q got %+v", asked, got)
	}
}

func TestShouldRejectForeignOriginsAndHosts(t *testing.T) {
	_, ts := newTestServer(t)
	cases := []struct {
		name, host, origin string
		want               int
	}{
		{"hook client without origin", "", "", http.StatusNoContent},
		{"same origin page", "", "self", http.StatusNoContent},
		{"other website", "", "https://evil.example", http.StatusForbidden},
		{"dns rebinding host", "evil.example:7337", "", http.StatusForbidden},
		{"localhost name", "localhost:7337", "http://localhost:7337", http.StatusNoContent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", ts.URL+"/event", strings.NewReader(`{"hook_event_name":"Stop","session_id":"x"}`))
			if c.host != "" {
				req.Host = c.host
			}
			if c.origin == "self" {
				req.Header.Set("Origin", ts.URL)
			} else if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			r, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			r.Body.Close()
			if r.StatusCode != c.want {
				t.Fatalf("got %d, want %d", r.StatusCode, c.want)
			}
		})
	}
}
