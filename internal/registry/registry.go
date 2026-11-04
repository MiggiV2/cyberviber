// Package registry reads Claude Code's own session registry (~/.claude/sessions/<pid>.json).
// The format is undocumented, so it is used best-effort to discover sessions that have not
// fired a hook yet; hooks stay the primary source. The *.key files there are never opened.
package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Entry struct {
	PID       int
	SessionID string
	Cwd       string
	Name      string // Claude Code's session name (derived or set via /rename)
	Status    string // "busy" | "idle" (others possible)
	StatusAt  time.Time
}

type file struct {
	PID             int    `json:"pid"`
	SessionID       string `json:"sessionId"`
	Cwd             string `json:"cwd"`
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Status          string `json:"status"`
	StatusUpdatedAt int64  `json:"statusUpdatedAt"`
}

func DefaultDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "sessions")
}

// Read returns all interactive sessions in dir. Unreadable or incomplete files are skipped.
func Read(dir string) []Entry {
	paths, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var out []Entry
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var f file
		if json.Unmarshal(b, &f) != nil || f.PID <= 0 || f.SessionID == "" || f.Kind != "interactive" {
			continue
		}
		out = append(out, Entry{PID: f.PID, SessionID: f.SessionID, Cwd: f.Cwd, Name: f.Name, Status: f.Status,
			StatusAt: time.UnixMilli(f.StatusUpdatedAt)})
	}
	return out
}
