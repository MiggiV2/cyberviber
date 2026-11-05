package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
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

func TestShouldValidateAndForwardConfig(t *testing.T) {
	api, ts := newTestServer(t)
	var got []protocol.Config
	api.Configure = func(c protocol.Config) error {
		got = append(got, c)
		return nil
	}
	post := func(body string) int {
		resp, err := http.Post(ts.URL+"/config", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if c := post(`{"layout":"tiling"}`); c != http.StatusNoContent {
		t.Fatalf("status = %d", c)
	}
	if c := post(`{"theme":"nord","layout":"btop"}`); c != http.StatusNoContent {
		t.Fatalf("status = %d", c)
	}
	for _, bad := range []string{`{"layout":"nope"}`, `{"theme":"nope"}`, `{}`, `x`} {
		if c := post(bad); c != http.StatusBadRequest {
			t.Fatalf("%s: status = %d", bad, c)
		}
	}
	want := []protocol.Config{{Type: "config", Layout: "tiling"}, {Type: "config", Layout: "btop", Theme: "nord"}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v", got)
	}

	api.Configure = func(protocol.Config) error { return errors.New("device not connected") }
	if c := post(`{"layout":"tiling"}`); c != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", c)
	}
}

func TestShouldValidateBacklightConfig(t *testing.T) {
	api, ts := newTestServer(t)
	var got []protocol.Config
	api.Configure = func(c protocol.Config) error {
		got = append(got, c)
		return nil
	}
	post := func(body string) int {
		resp, err := http.Post(ts.URL+"/config", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	for _, ok := range []string{`{"brightness":40}`, `{"dim_after":0}`, `{"dim_after":15,"brightness":100}`} {
		if c := post(ok); c != http.StatusNoContent {
			t.Fatalf("%s: status = %d", ok, c)
		}
	}
	for _, bad := range []string{`{"brightness":101}`, `{"brightness":-1}`, `{"dim_after":-1}`, `{"dim_after":1441}`} {
		if c := post(bad); c != http.StatusBadRequest {
			t.Fatalf("%s: status = %d", bad, c)
		}
	}
	if len(got) != 3 || got[0].Brightness != 40 || got[0].DimAfter != nil ||
		got[1].DimAfter == nil || *got[1].DimAfter != 0 || got[2].Brightness != 100 || *got[2].DimAfter != 15 {
		t.Fatalf("got %+v", got)
	}
	b, _ := json.Marshal(got[1])
	if string(b) != `{"type":"config","dim_after":0}` {
		t.Fatalf("dim_after 0 must reach the device: %s", b)
	}
}

func TestShouldStoreContextUsage(t *testing.T) {
	api, ts := newTestServer(t)
	post := func(path, body string) int {
		resp, err := http.Post(ts.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if c := post("/event", `{"session_id":"s1","hook_event_name":"SessionStart","cwd":"/tmp/x"}`); c != http.StatusNoContent {
		t.Fatalf("event: %d", c)
	}
	if c := post("/ctx", `{"session_id":"s1","pct":47}`); c != http.StatusNoContent {
		t.Fatalf("ctx: %d", c)
	}
	var snap protocol.Sessions
	if err := json.Unmarshal(api.Snapshot(), &snap); err != nil || len(snap.Sessions) != 1 || snap.Sessions[0].Ctx != 47 {
		t.Fatalf("snapshot = %+v (%v)", snap, err)
	}
	for _, bad := range []string{`{"session_id":"s1","pct":101}`, `{"pct":5}`, `x`} {
		if c := post("/ctx", bad); c != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, c)
		}
	}
	if c := post("/ctx", `{"session_id":"unknown","pct":5}`); c != http.StatusNoContent {
		t.Fatalf("unknown session must be ignored quietly: %d", c)
	}
}
