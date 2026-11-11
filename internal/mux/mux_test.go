package mux

import "testing"

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestShouldDetectZellij(t *testing.T) {
	// given
	getenv := env(map[string]string{"ZELLIJ_SESSION_NAME": "sincere-drum", "ZELLIJ_PANE_ID": "23"})

	// when
	loc, ok := Detect(getenv)

	// then
	if !ok || loc != (Location{Kind: "zellij", Session: "sincere-drum", Pane: "23"}) || loc.Label() != "sincere-drum" || loc.Short() != "zj" {
		t.Fatalf("ok=%v %+v", ok, loc)
	}
}

func TestShouldDetectNothingOutsideMultiplexers(t *testing.T) {
	if loc, ok := Detect(env(map[string]string{"TERM_PROGRAM": "zellij"})); ok || !loc.IsZero() {
		t.Fatalf("ok=%v %+v", ok, loc)
	}
}

func TestShouldListKinds(t *testing.T) {
	if k := Kinds(); len(k) == 0 || k[0] != "zellij" {
		t.Fatalf("%v", k)
	}
}

func TestShouldDetectHerdrWithSessionLabel(t *testing.T) {
	// given
	getenv := env(map[string]string{"HERDR_SOCKET_PATH": "/Users/m/.config/herdr/herdr.sock", "HERDR_PANE_ID": "w1M:p1"})

	// when
	loc, ok := Detect(getenv)

	// then
	if !ok || loc != (Location{Kind: "herdr", Session: "/Users/m/.config/herdr/herdr.sock", Pane: "w1M:p1"}) ||
		loc.Label() != "default" || loc.Short() != "hd" {
		t.Fatalf("ok=%v %+v label=%s", ok, loc, loc.Label())
	}
	if named := (Location{Kind: "herdr", Session: "/x/sessions/work/herdr.sock"}); named.Label() != "work" {
		t.Fatalf("label=%s", named.Label())
	}
}
