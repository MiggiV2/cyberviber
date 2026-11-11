// Package focus brings the terminal of a Claude Code session to the front:
// multiplexer pane (if any) → terminal tab → app activation.
//
// Adding a multiplexer: detect it in package mux, implement Multiplexer, register it in multiplexers.
// Adding a terminal app: implement App, register it in apps under the lower-cased app bundle name.
package focus

import (
	"strings"

	"github.com/d135-1r43/cyberviber/internal/proc"
)

// Terminal is the terminal app hosting an anchor process.
type Terminal struct {
	App    string // lower-cased app bundle name, e.g. "tabby", "ghostty"
	Bundle string // e.g. /Applications/Tabby.app
	AppPID int
	Chain  []int  // anchor … tab root (the process right below the app)
	TTY    string // the anchor's TTY (outer TTY)
}

// ResolveTerminal walks up from the anchor process until its parent is a macOS app bundle.
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
		if bundle, ok := appBundle(parent.Exe); ok {
			t.Bundle, t.AppPID = bundle, parent.PID
			name := strings.TrimSuffix(bundle[strings.LastIndex(bundle, "/")+1:], ".app")
			t.App = strings.ToLower(name)
			return t, true
		}
		pid = p.PPID
	}
	return Terminal{}, false
}

func appBundle(exe string) (string, bool) {
	i := strings.Index(exe, ".app/Contents/MacOS/")
	if i < 0 {
		return "", false
	}
	return exe[:i+len(".app")], true
}
