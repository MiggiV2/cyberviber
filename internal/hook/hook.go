// Package hook implements `cyberviber hook`: read a Claude Code hook payload, enrich it, forward it.
package hook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"cyberviber/internal/event"
	"cyberviber/internal/proc"
)

type payload struct {
	event.Event
	ToolInput json.RawMessage `json:"tool_input"`
}

// Build parses the hook payload from r and adds activity text and process information.
func Build(r io.Reader, getenv func(string) string, agent func() (proc.Proc, bool), now time.Time) (event.Event, error) {
	var p payload
	if err := json.NewDecoder(io.LimitReader(r, 16<<20)).Decode(&p); err != nil {
		return event.Event{}, fmt.Errorf("parse hook payload: %w", err)
	}
	e := p.Event
	e.Time = now
	switch e.Name {
	case event.PreToolUse:
		e.Activity, e.File = event.Summarize(e.ToolName, p.ToolInput)
	case event.PermissionRequest:
		act, _ := event.Summarize(e.ToolName, p.ToolInput)
		e.Activity = "Allow " + act + "?"
	}
	if a, ok := agent(); ok {
		e.PID, e.TTY = a.PID, a.TTY
	}
	e.ZellijSession = getenv("ZELLIJ_SESSION_NAME")
	e.ZellijPane = getenv("ZELLIJ_PANE_ID")
	return e, nil
}

// Send posts the event to the daemon. It never blocks for long: hooks must not slow Claude Code down.
func Send(port int, e event.Event) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	c := http.Client{Timeout: 500 * time.Millisecond}
	resp, err := c.Post(fmt.Sprintf("http://127.0.0.1:%d/event", port), "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
