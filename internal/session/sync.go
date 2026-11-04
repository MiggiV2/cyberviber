package session

import (
	"path/filepath"
	"time"
)

// Discovered is a session found outside the hooks (Claude Code's session registry).
type Discovered struct {
	ID       string
	Cwd      string
	Name     string // Claude Code session name; empty → folder name
	PID      int
	TTY      string
	Status   string // "busy" | "idle"
	StatusAt time.Time

	ZellijSession, ZellijPane string // from the process environment
}

// Sync adds sessions that have not fired a hook yet and applies registry status changes that
// are newer than the last hook event. Only hooks set or clear Input. Reports whether anything changed.
func (s *Store) Sync(list []Discovered) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for _, d := range list {
		ss, ok := s.sessions[d.ID]
		if !ok {
			ss = &Session{ID: d.ID, Cwd: d.Cwd, Name: filepath.Base(d.Cwd), PID: d.PID, TTY: d.TTY,
				State: Ready, Activity: "Waiting for prompt", Since: d.StatusAt, LastEvent: d.StatusAt}
			if d.Status == "busy" {
				ss.State, ss.Activity = Working, "Working…"
			}
			if d.Name != "" {
				ss.Name = d.Name
			}
			ss.ZellijSession, ss.ZellijPane = d.ZellijSession, d.ZellijPane
			ss.Branch = s.branchOf(d.Cwd)
			s.sessions[d.ID] = ss
			changed = true
			continue
		}
		if d.Name != "" && ss.Name != d.Name {
			ss.Name = d.Name
			changed = true
		}
		if ss.ZellijSession == "" && d.ZellijSession != "" {
			ss.ZellijSession, ss.ZellijPane = d.ZellijSession, d.ZellijPane
			changed = true
		}
		if ss.PID == 0 && d.PID != 0 {
			ss.PID, ss.TTY = d.PID, d.TTY
			changed = true
		}
		if !d.StatusAt.After(ss.LastEvent) || ss.State == Input {
			continue
		}
		switch {
		case d.Status == "busy" && (ss.State == Ready || ss.State == Idle):
			ss.State, ss.Since, ss.Activity = Working, d.StatusAt, "Working…"
			ss.Edited = nil
		case d.Status == "idle" && ss.State == Working:
			ss.State, ss.Since, ss.Activity = Ready, d.StatusAt, doneText(len(ss.Edited))
		default:
			continue
		}
		ss.LastEvent = d.StatusAt
		changed = true
	}
	return changed
}
