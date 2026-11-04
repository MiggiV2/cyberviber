package hook

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cyberviber/internal/event"
	"cyberviber/internal/proc"
)

func TestShouldEnrichPreToolUse(t *testing.T) {
	// given
	in := `{"session_id":"s1","cwd":"/x","hook_event_name":"PreToolUse","tool_name":"Edit",
	        "tool_input":{"file_path":"/x/main.go","old_string":"a","new_string":"b"}}`
	env := map[string]string{"ZELLIJ_SESSION_NAME": "sincere-drum", "ZELLIJ_PANE_ID": "23"}
	agent := func() (proc.Proc, bool) { return proc.Proc{PID: 80, TTY: "ttys022"}, true }
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)

	// when
	e, err := Build(strings.NewReader(in), func(k string) string { return env[k] }, agent, now)

	// then
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != event.PreToolUse || e.Activity != "Edit main.go" || e.File != "/x/main.go" ||
		e.PID != 80 || e.TTY != "ttys022" || e.ZellijSession != "sincere-drum" || e.ZellijPane != "23" || !e.Time.Equal(now) {
		t.Fatalf("unexpected %+v", e)
	}
	b, _ := json.Marshal(e)
	if strings.Contains(string(b), "old_string") {
		t.Fatal("raw tool input must not be forwarded")
	}
}

func TestShouldPhrasePermissionRequestAsQuestion(t *testing.T) {
	in := `{"session_id":"s1","hook_event_name":"PermissionRequest","tool_name":"Bash","tool_input":{"command":"rm -rf build"}}`
	e, err := Build(strings.NewReader(in), func(string) string { return "" }, func() (proc.Proc, bool) { return proc.Proc{}, false }, time.Now())
	if err != nil || e.Activity != "Allow Bash: rm -rf build?" || e.PID != 0 {
		t.Fatalf("err=%v %+v", err, e)
	}
}

func TestShouldFailOnGarbage(t *testing.T) {
	_, err := Build(strings.NewReader("nope"), func(string) string { return "" }, func() (proc.Proc, bool) { return proc.Proc{}, false }, time.Now())
	if err == nil {
		t.Fatal("expected error")
	}
}
