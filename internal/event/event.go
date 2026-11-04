// Package event defines the hook event forwarded from `cyberviber hook` to the daemon.
package event

import "time"

// Claude Code hook event names.
const (
	SessionStart       = "SessionStart"
	SessionEnd         = "SessionEnd"
	UserPromptSubmit   = "UserPromptSubmit"
	PreToolUse         = "PreToolUse"
	PostToolUse        = "PostToolUse"
	PostToolUseFailure = "PostToolUseFailure"
	PermissionRequest  = "PermissionRequest"
	Notification       = "Notification"
	Stop               = "Stop"
)

// Event is a Claude Code hook payload reduced to what the daemon needs, plus process
// information added by the hook command. Raw tool input is never forwarded.
type Event struct {
	Name             string `json:"hook_event_name"`
	SessionID        string `json:"session_id"`
	Cwd              string `json:"cwd,omitempty"`
	TranscriptPath   string `json:"transcript_path,omitempty"`
	ToolName         string `json:"tool_name,omitempty"`
	Message          string `json:"message,omitempty"`
	NotificationType string `json:"notification_type,omitempty"`
	Source           string `json:"source,omitempty"`

	// Added by `cyberviber hook`.
	Activity      string    `json:"cv_activity,omitempty"`
	File          string    `json:"cv_file,omitempty"`
	PID           int       `json:"cv_pid,omitempty"`
	TTY           string    `json:"cv_tty,omitempty"`
	ZellijSession string    `json:"cv_zellij_session,omitempty"`
	ZellijPane    string    `json:"cv_zellij_pane,omitempty"`
	Time          time.Time `json:"cv_time"`
}
