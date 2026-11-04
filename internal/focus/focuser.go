package focus

import (
	"errors"
	"log"
	"sync"

	"cyberviber/internal/proc"
	"cyberviber/internal/session"
)

// Result of a focus request. Reason is set when OK is false.
type Result struct {
	OK       bool
	Reason   string // not-found, no-process, detached, unknown-terminal, no-plugin, tab-not-found
	Terminal string
	Zellij   string
	Tab      int
}

type Focuser struct {
	Lookup   func(id string) (session.Session, bool)
	Get      func(pid int) (proc.Proc, error)
	Env      func(pid int) map[string]string
	Run      func(name string, args ...string) ([]byte, error)
	TabbyDir string

	mu         sync.Mutex
	ghosttyIDs map[string]string // session id → Ghostty terminal id
}

// Focus brings the session's terminal to the front.
func (f *Focuser) Focus(id string) Result {
	s, ok := f.Lookup(id)
	if !ok {
		return Result{Reason: "not-found"}
	}
	if s.PID == 0 {
		return Result{Reason: "no-process"}
	}

	zs, zp := s.ZellijSession, s.ZellijPane
	if zs == "" {
		env := f.Env(s.PID)
		zs, zp = env["ZELLIJ_SESSION_NAME"], env["ZELLIJ_PANE_ID"]
	}
	res := Result{Zellij: zs}

	anchor := s.PID
	if zs != "" {
		out, err := f.Run("lsof", "-U", "-a", "-c", "zellij", "-F", "pfdn")
		if err != nil && len(out) == 0 {
			log.Printf("focus %s: lsof: %v", id, err)
		}
		clients := ParseLsof(string(out))[zs]
		zellij := ""
		if len(clients) > 0 {
			anchor = clients[0]
			if c, err := f.Get(anchor); err == nil {
				zellij = c.Exe // same binary (and version) as the attached client
			}
		}
		if zellij == "" {
			zellij = lookZellij()
		}
		if zp != "" {
			if out, err := f.Run(zellij, "--session", zs, "action", "focus-pane-id", paneID(zp)); err != nil {
				log.Printf("focus %s: zellij focus-pane-id: %v %s", id, err, out)
			}
		}
		if len(clients) == 0 {
			res.Reason = "detached"
			return res
		}
	}

	term, ok := ResolveTerminal(anchor, f.Get)
	if !ok {
		res.Reason = "unknown-terminal"
		return res
	}
	res.Terminal = term.App

	switch term.App {
	case "tabby":
		tab, err := FocusTabby(f.TabbyDir, term.Chain)
		f.Run("open", "-a", term.Bundle)
		switch {
		case errors.Is(err, ErrNoPlugin):
			res.Reason = "no-plugin"
		case err != nil:
			res.Reason = "tab-not-found"
		default:
			res.OK, res.Tab = true, tab
		}
	case "ghostty":
		if f.focusGhostty(s.ID, s.Cwd, term.TTY, zs != "") {
			res.OK = true
		} else {
			f.Run("open", "-a", term.Bundle)
			res.Reason = "tab-not-found"
		}
	default:
		f.Run("open", "-a", term.Bundle)
		res.Reason = "unknown-terminal"
	}
	return res
}
