// Package session keeps the state of all Claude Code sessions and implements the state machine.
package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/d135-1r43/cyberviber/internal/event"
	"github.com/d135-1r43/cyberviber/internal/mux"
)

type State string

const (
	Input   State = "input"
	Ready   State = "ready"
	Working State = "working"
	Idle    State = "idle"
)

var priority = map[State]int{Input: 0, Ready: 1, Working: 2, Idle: 3}

// orphanAfter drops sessions without a known PID after this much silence.
const orphanAfter = 12 * time.Hour

type Session struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Cwd       string       `json:"cwd"`
	Branch    string       `json:"branch,omitempty"`
	State     State        `json:"state"`
	Activity  string       `json:"activity"`
	Since     time.Time    `json:"since"`
	LastEvent time.Time    `json:"last_event"`
	PID       int          `json:"pid,omitempty"`
	TTY       string       `json:"tty,omitempty"`
	Mux       mux.Location `json:"mux,omitzero"`
	Terminal  string       `json:"terminal,omitempty"`
	Tab       int          `json:"tab,omitempty"`
	Ctx       int          `json:"ctx,omitempty"`
	Edited    []string     `json:"edited,omitempty"`
}

type Store struct {
	mu        sync.Mutex
	sessions  map[string]*Session
	idleAfter time.Duration
	branchOf  func(cwd string) string
}

func New(idleAfter time.Duration, branchOf func(cwd string) string) *Store {
	return &Store{sessions: map[string]*Session{}, idleAfter: idleAfter, branchOf: branchOf}
}

// Apply updates the store with one hook event.
func (s *Store) Apply(e event.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if e.Name == event.SessionEnd {
		delete(s.sessions, e.SessionID)
		return
	}
	ss, ok := s.sessions[e.SessionID]
	if !ok {
		ss = &Session{ID: e.SessionID, Cwd: e.Cwd, Name: filepath.Base(e.Cwd), Since: e.Time, State: Ready}
		ss.Branch = s.branchOf(e.Cwd)
		s.sessions[e.SessionID] = ss
	}
	ss.LastEvent = e.Time
	if e.PID != 0 {
		ss.PID, ss.TTY = e.PID, e.TTY
		s.dropOthersOf(e.PID, e.SessionID)
	}
	if !e.Mux.IsZero() {
		ss.Mux = e.Mux
	}

	set := func(st State, activity string) {
		if ss.State != st {
			ss.State, ss.Since = st, e.Time
		}
		if activity != "" {
			ss.Activity = activity
		}
	}

	switch e.Name {
	case event.SessionStart:
		ss.Branch = s.branchOf(ss.Cwd)
		set(Ready, "New session")
	case event.UserPromptSubmit:
		ss.Branch = s.branchOf(ss.Cwd)
		ss.Edited = nil
		set(Working, "Thinking…")
	case event.PreToolUse:
		if e.ToolName == "AskUserQuestion" {
			set(Input, e.Activity)
			break
		}
		if e.File != "" && !contains(ss.Edited, e.File) {
			ss.Edited = append(ss.Edited, e.File)
		}
		set(Working, e.Activity)
	case event.PostToolUse, event.PostToolUseFailure:
		set(Working, "")
	case event.PermissionRequest:
		set(Input, e.Activity)
	case event.Notification:
		if needsInput(e) && ss.State != Input {
			set(Input, e.Message)
		}
	case event.Stop:
		ss.Branch = s.branchOf(ss.Cwd)
		set(Ready, doneText(len(ss.Edited)))
	}
}

// dropOthersOf removes sessions a process has left, e.g. the picker session of `claude --resume`.
func (s *Store) dropOthersOf(pid int, keep string) bool {
	dropped := false
	for id, ss := range s.sessions {
		if id != keep && ss.PID == pid {
			delete(s.sessions, id)
			dropped = true
		}
	}
	return dropped
}

func needsInput(e event.Event) bool {
	switch e.NotificationType {
	case "permission_prompt", "elicitation_dialog":
		return true
	case "":
		return containsFold(e.Message, "permission")
	}
	return false
}

func doneText(n int) string {
	switch n {
	case 0:
		return "Done"
	case 1:
		return "Done · 1 file changed"
	}
	return fmt.Sprintf("Done · %d files changed", n)
}

// Tick applies time-based transitions and drops dead sessions. It reports whether anything changed.
func (s *Store) Tick(now time.Time, alive func(pid int) bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for id, ss := range s.sessions {
		if ss.PID != 0 && !alive(ss.PID) || ss.PID == 0 && now.Sub(ss.LastEvent) > orphanAfter {
			delete(s.sessions, id)
			changed = true
			continue
		}
		if ss.State == Ready && now.Sub(ss.Since) >= s.idleAfter {
			ss.State = Idle // Since stays: shows how long the session has been untouched
			changed = true
		}
	}
	return changed
}

// Get returns one session.
func (s *Store) Get(id string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.sessions[id]
	if !ok {
		return Session{}, false
	}
	return *ss, true
}

// SetTerminal records where a session's terminal was found (shown on the device).
func (s *Store) SetTerminal(id, terminal string, tab int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.sessions[id]
	if !ok || ss.Terminal == terminal && ss.Tab == tab {
		return false
	}
	ss.Terminal, ss.Tab = terminal, tab
	return true
}

// SetCtx records a session's context usage in percent; unknown sessions are ignored.
func (s *Store) SetCtx(id string, pct int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.sessions[id]
	if !ok || ss.Ctx == pct {
		return false
	}
	ss.Ctx = pct
	return true
}

// List returns all sessions: needs input → ready → working → idle, then most recent first.
func (s *Store) List() []Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Session, 0, len(s.sessions))
	for _, ss := range s.sessions {
		c := *ss
		c.Edited = append([]string(nil), ss.Edited...)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if pi, pj := priority[out[i].State], priority[out[j].State]; pi != pj {
			return pi < pj
		}
		if !out[i].Since.Equal(out[j].Since) {
			return out[i].Since.After(out[j].Since)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Save writes all sessions atomically to path.
func (s *Store) Save(path string) error {
	b, err := json.MarshalIndent(s.List(), "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load restores sessions written by Save. A missing file is not an error.
func (s *Store) Load(path string) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var list []Session
	if err := json.Unmarshal(b, &list); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range list {
		s.sessions[list[i].ID] = &list[i]
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
