// Package focus brings the terminal of a Claude Code session to the front:
// zellij pane (if any) → terminal tab (Tabby via plugin, Ghostty via AppleScript) → app activation.
package focus

import (
	"strings"

	"cyberviber/internal/proc"
)

// Terminal is the terminal app hosting an anchor process.
type Terminal struct {
	App    string // "tabby", "ghostty" or the lower-cased app name for other apps
	Bundle string // e.g. /Applications/Tabby.app
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
			t.Bundle = bundle
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
