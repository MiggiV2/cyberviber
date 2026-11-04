package session

import (
	"path/filepath"
	"testing"
	"time"

	"cyberviber/internal/event"
)

var t0 = time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)

func newStore() *Store {
	return New(10*time.Minute, func(string) string { return "main" })
}

func ev(name string, at time.Duration) event.Event {
	return event.Event{Name: name, SessionID: "s1", Cwd: "/Users/m/git/cyberviber", Time: t0.Add(at), PID: 4242}
}

func only(t *testing.T, s *Store) Session {
	t.Helper()
	l := s.List()
	if len(l) != 1 {
		t.Fatalf("got %d sessions, want 1", len(l))
	}
	return l[0]
}

func TestShouldCreateReadySessionOnStart(t *testing.T) {
	// given
	s := newStore()

	// when
	s.Apply(ev(event.SessionStart, 0))

	// then
	got := only(t, s)
	if got.State != Ready || got.Name != "cyberviber" || got.Branch != "main" || got.PID != 4242 {
		t.Fatalf("unexpected session %+v", got)
	}
}

func TestShouldCreateSessionFromAnyEventWhenStartWasMissed(t *testing.T) {
	// given
	s := newStore()
	e := ev(event.PreToolUse, 0)
	e.Activity = "Edit main.go"

	// when
	s.Apply(e)

	// then
	got := only(t, s)
	if got.State != Working || got.Activity != "Edit main.go" {
		t.Fatalf("unexpected session %+v", got)
	}
}

func TestShouldFollowATypicalTurn(t *testing.T) {
	// given
	s := newStore()
	s.Apply(ev(event.SessionStart, 0))

	// when / then
	s.Apply(ev(event.UserPromptSubmit, time.Second))
	if got := only(t, s); got.State != Working || !got.Since.Equal(t0.Add(time.Second)) {
		t.Fatalf("after prompt: %+v", got)
	}

	tool := ev(event.PreToolUse, 2*time.Second)
	tool.Activity, tool.File = "Edit a.go", "/x/a.go"
	s.Apply(tool)
	tool2 := ev(event.PreToolUse, 3*time.Second)
	tool2.Activity, tool2.File = "Edit b.go", "/x/b.go"
	s.Apply(tool2)
	s.Apply(tool) // same file twice counts once
	if got := only(t, s); got.State != Working || got.Activity != "Edit a.go" || !got.Since.Equal(t0.Add(time.Second)) {
		t.Fatalf("working should keep since of prompt: %+v", got)
	}

	s.Apply(ev(event.Stop, 10*time.Second))
	if got := only(t, s); got.State != Ready || got.Activity != "Done · 2 files changed" {
		t.Fatalf("after stop: %+v", got)
	}

	s.Apply(ev(event.UserPromptSubmit, 20*time.Second))
	s.Apply(ev(event.Stop, 21*time.Second))
	if got := only(t, s); got.Activity != "Done" {
		t.Fatalf("edited files must reset per turn: %+v", got)
	}
}

func TestShouldNeedInputOnPermissionRequestAndResumeAfterwards(t *testing.T) {
	// given
	s := newStore()
	s.Apply(ev(event.UserPromptSubmit, 0))
	p := ev(event.PermissionRequest, time.Second)
	p.Activity = "Allow Bash: rm -rf build?"

	// when
	s.Apply(p)

	// then
	if got := only(t, s); got.State != Input || got.Activity != "Allow Bash: rm -rf build?" {
		t.Fatalf("after permission request: %+v", got)
	}

	// when the permission notification follows, the more specific text stays
	n := ev(event.Notification, 2*time.Second)
	n.NotificationType, n.Message = "permission_prompt", "Claude needs your permission to use Bash"
	s.Apply(n)
	if got := only(t, s); got.Activity != "Allow Bash: rm -rf build?" || !got.Since.Equal(t0.Add(time.Second)) {
		t.Fatalf("notification must not override: %+v", got)
	}

	// when the tool ran
	s.Apply(ev(event.PostToolUse, 5*time.Second))
	if got := only(t, s); got.State != Working {
		t.Fatalf("after post tool use: %+v", got)
	}
}

func TestShouldNeedInputOnPermissionNotificationAlone(t *testing.T) {
	// given
	s := newStore()
	s.Apply(ev(event.UserPromptSubmit, 0))
	n := ev(event.Notification, time.Second)
	n.NotificationType, n.Message = "permission_prompt", "Claude needs your permission to use Bash"

	// when
	s.Apply(n)

	// then
	if got := only(t, s); got.State != Input || got.Activity != n.Message {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestShouldIgnoreIdleNotification(t *testing.T) {
	// given
	s := newStore()
	s.Apply(ev(event.Stop, 0))
	n := ev(event.Notification, time.Minute)
	n.NotificationType, n.Message = "idle_prompt", "Claude is waiting for your input"

	// when
	s.Apply(n)

	// then
	if got := only(t, s); got.State != Ready || !got.Since.Equal(t0) {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestShouldNeedInputWhenAskingAQuestion(t *testing.T) {
	// given
	s := newStore()
	s.Apply(ev(event.UserPromptSubmit, 0))
	q := ev(event.PreToolUse, time.Second)
	q.ToolName, q.Activity = "AskUserQuestion", "Which version?"

	// when
	s.Apply(q)

	// then
	if got := only(t, s); got.State != Input || got.Activity != "Which version?" {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestShouldRemoveSessionOnEnd(t *testing.T) {
	// given
	s := newStore()
	s.Apply(ev(event.SessionStart, 0))

	// when
	s.Apply(ev(event.SessionEnd, time.Second))

	// then
	if l := s.List(); len(l) != 0 {
		t.Fatalf("got %d sessions", len(l))
	}
}

func TestShouldGoIdleAfterThresholdOnlyWhenReady(t *testing.T) {
	// given
	s := newStore()
	s.Apply(ev(event.Stop, 0))
	alive := func(int) bool { return true }

	// when / then
	if s.Tick(t0.Add(9*time.Minute), alive) {
		t.Fatal("no change expected before threshold")
	}
	if !s.Tick(t0.Add(11*time.Minute), alive) {
		t.Fatal("change expected after threshold")
	}
	if got := only(t, s); got.State != Idle || !got.Since.Equal(t0) {
		t.Fatalf("idle must keep since of last activity: %+v", got)
	}

	// working never goes idle
	s.Apply(ev(event.UserPromptSubmit, 12*time.Minute))
	s.Tick(t0.Add(2*time.Hour), alive)
	if got := only(t, s); got.State != Working {
		t.Fatalf("working must stay working: %+v", got)
	}
}

func TestShouldDropDeadSessions(t *testing.T) {
	// given
	s := newStore()
	s.Apply(ev(event.SessionStart, 0))

	// when
	changed := s.Tick(t0.Add(time.Second), func(pid int) bool { return pid != 4242 })

	// then
	if !changed || len(s.List()) != 0 {
		t.Fatal("dead session must be dropped")
	}
}

func TestShouldDropSessionsWithoutPidAfterLongSilence(t *testing.T) {
	// given
	s := newStore()
	e := ev(event.SessionStart, 0)
	e.PID = 0
	s.Apply(e)
	alive := func(int) bool { t.Fatal("must not check pid 0"); return false }

	// when / then
	s.Tick(t0.Add(time.Hour), alive)
	if len(s.List()) != 1 {
		t.Fatal("dropped too early")
	}
	s.Tick(t0.Add(13*time.Hour), alive)
	if len(s.List()) != 0 {
		t.Fatal("must be dropped after 12h")
	}
}

func TestShouldSortByPriorityThenMostRecent(t *testing.T) {
	// given
	s := newStore()
	add := func(id, name string, at time.Duration) {
		e := ev(name, at)
		e.SessionID, e.Cwd = id, "/p/"+id
		s.Apply(e)
	}
	add("working-old", event.UserPromptSubmit, 1*time.Second)
	add("ready", event.Stop, 2*time.Second)
	add("working-new", event.UserPromptSubmit, 3*time.Second)
	add("input", event.PermissionRequest, 0)

	// when
	l := s.List()

	// then
	want := []string{"input", "ready", "working-new", "working-old"}
	for i, w := range want {
		if l[i].ID != w {
			t.Fatalf("pos %d = %s, want %s (%v)", i, l[i].ID, w, l)
		}
	}
}

func TestShouldPersistAndRestore(t *testing.T) {
	// given
	s := newStore()
	e := ev(event.SessionStart, 0)
	e.ZellijSession, e.ZellijPane = "sincere-drum", "23"
	s.Apply(e)
	path := filepath.Join(t.TempDir(), "sessions.json")

	// when
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	r := newStore()
	if err := r.Load(path); err != nil {
		t.Fatal(err)
	}

	// then
	got := only(t, r)
	if got.ID != "s1" || got.ZellijSession != "sincere-drum" || got.ZellijPane != "23" || got.State != Ready {
		t.Fatalf("restored %+v", got)
	}
}

func TestShouldLoadMissingFileAsEmpty(t *testing.T) {
	s := newStore()
	if err := s.Load(filepath.Join(t.TempDir(), "nope.json")); err != nil {
		t.Fatal(err)
	}
}
