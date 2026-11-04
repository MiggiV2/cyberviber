// Package protocol defines the JSON messages between daemon and device (one message per line).
package protocol

import "cyberviber/internal/session"

type Session struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Branch   string `json:"branch,omitempty"`
	State    string `json:"state"`
	Activity string `json:"activity"`
	Since    int64  `json:"since"`
	Ctx      int    `json:"ctx,omitempty"`
	Terminal string `json:"terminal,omitempty"`
	Tab      int    `json:"tab,omitempty"`
	Zellij   string `json:"zellij,omitempty"`
}

type Sessions struct {
	Type     string    `json:"type"`
	Now      int64     `json:"now"`
	Sessions []Session `json:"sessions"`
}

func FromSessions(now int64, list []session.Session) Sessions {
	out := Sessions{Type: "sessions", Now: now, Sessions: make([]Session, 0, len(list))}
	for _, s := range list {
		out.Sessions = append(out.Sessions, Session{
			ID: s.ID, Name: s.Name, Branch: s.Branch, State: string(s.State), Activity: s.Activity,
			Since: s.Since.Unix(), Ctx: s.Ctx, Terminal: s.Terminal, Tab: s.Tab, Zellij: s.ZellijSession,
		})
	}
	return out
}
