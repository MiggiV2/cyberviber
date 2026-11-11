// Package focus brings the terminal of a Claude Code session to the front:
// multiplexer pane (if any) → terminal tab → app activation.
//
// Adding a multiplexer: detect it in package mux, implement Multiplexer, register it in multiplexers.
// Adding a terminal app: implement App, register it in apps under the lower-cased app bundle name.
package focus

import (
	"path/filepath"
	"strings"

	"github.com/d135-1r43/cyberviber/internal/proc"
)

// Terminal is the terminal app hosting an anchor process.
type Terminal struct {
	App    string // lower-cased app bundle name, e.g. "tabby", "ghostty"
	Bundle string // macOS: /Applications/Tabby.app; Linux: the executable, e.g. /usr/bin/kitty
	AppPID int
	Chain  []int  // anchor … tab root (the process right below the app)
	TTY    string // the anchor's TTY (outer TTY)
}

// ResolveTerminal walks up from the anchor process until its parent is a terminal app.
func ResolveTerminal(anchor int, get func(int) (proc.Proc, error)) (Terminal, bool) {
	var t Terminal
	pid := anchor
	for i := 0; i < 64 && pid > 1; i++ {
		p, err := get(pid)
		if err != nil {
			return Terminal{}, false
		}
		if i == 0 {
			t.TTY = p.TTY
		}
		t.Chain = append(t.Chain, p.PID)
		parent, err := get(p.PPID)
		if err != nil {
			return Terminal{}, false
		}
		if app, bundle, ok := appOf(parent.Exe); ok {
			t.App, t.Bundle, t.AppPID = app, bundle, parent.PID
			return t, true
		}
		pid = p.PPID
	}
	return Terminal{}, false
}

// linuxTerminals are terminal emulators recognised by executable name; macOS apps by their bundle.
var linuxTerminals = map[string]bool{"kitty": true, "ghostty": true, "tabby": true, "alacritty": true,
	"foot": true, "wezterm-gui": true, "konsole": true, "gnome-terminal-server": true, "xterm": true}

func appOf(exe string) (app, bundle string, ok bool) {
	if i := strings.Index(exe, ".app/Contents/MacOS/"); i >= 0 {
		bundle = exe[:i+len(".app")]
		return strings.ToLower(strings.TrimSuffix(filepath.Base(bundle), ".app")), bundle, true
	}
	if name := filepath.Base(exe); linuxTerminals[name] {
		return name, exe, true
	}
	return "", "", false
}
