// Package mux identifies the terminal multiplexer a process runs in, from its environment.
// Adding a multiplexer: one entry in kinds here, one Multiplexer in package focus.
package mux

import "path/filepath"

// Location is where a session lives inside a multiplexer.
type Location struct {
	Kind    string `json:"kind"`
	Session string `json:"session"`
	Pane    string `json:"pane,omitempty"`
}

type kind struct {
	name       string
	short      string
	sessionVar string
	paneVar    string
	label      func(session string) string
}

var kinds = []kind{
	{name: "zellij", short: "zj", sessionVar: "ZELLIJ_SESSION_NAME", paneVar: "ZELLIJ_PANE_ID"},
	{name: "herdr", short: "hd", sessionVar: "HERDR_SOCKET_PATH", paneVar: "HERDR_PANE_ID", label: herdrSession},
}

// herdrSession names a herdr session by its socket's directory; the default one lives in ~/.config/herdr.
func herdrSession(socket string) string {
	if dir := filepath.Base(filepath.Dir(socket)); dir != "herdr" {
		return dir
	}
	return "default"
}

// Detect reads the multiplexer location from a process environment.
func Detect(getenv func(string) string) (Location, bool) {
	for _, k := range kinds {
		if s := getenv(k.sessionVar); s != "" {
			return Location{Kind: k.name, Session: s, Pane: getenv(k.paneVar)}, true
		}
	}
	return Location{}, false
}

// Kinds lists the multiplexers Detect knows.
func Kinds() []string {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = k.name
	}
	return out
}

func (l Location) IsZero() bool { return l == Location{} }

// Short is the two-letter tag shown in compact layouts.
func (l Location) Short() string {
	for _, k := range kinds {
		if k.name == l.Kind {
			return k.short
		}
	}
	return ""
}

// Label is the short session name shown on the device.
func (l Location) Label() string {
	for _, k := range kinds {
		if k.name == l.Kind && k.label != nil {
			return k.label(l.Session)
		}
	}
	return l.Session
}
