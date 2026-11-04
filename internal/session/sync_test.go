package session

import (
	"testing"
	"time"

	"cyberviber/internal/event"
)

func disc(status string, at time.Duration) Discovered {
	return Discovered{ID: "s1", Cwd: "/Users/m/git/field-proof", PID: 20169, TTY: "ttys012", Status: status, StatusAt: t0.Add(at)}
}

func TestShouldAddUnknownSessionsFromRegistry(t *testing.T) {
	// given
	s := newStore()

	// when
	changed := s.Sync([]Discovered{disc("idle", 0)})

	// then
	got := only(t, s)
	if !changed || got.State != Ready || got.Name != "field-proof" || got.PID != 20169 || got.TTY != "ttys012" ||
		!got.Since.Equal(t0) || got.Branch != "main" {
		t.Fatalf("changed=%v %+v", changed, got)
	}
}

func TestShouldAddBusySessionAsWorking(t *testing.T) {
	s := newStore()
	s.Sync([]Discovered{disc("busy", 0)})
	if got := only(t, s); got.State != Working {
		t.Fatalf("%+v", got)
	}
}

func TestShouldNotReportChangeWhenNothingChanged(t *testing.T) {
	s := newStore()
	s.Sync([]Discovered{disc("idle", 0)})
	if s.Sync([]Discovered{disc("idle", 0)}) {
		t.Fatal("unexpected change")
	}
}

func TestShouldApplyNewerRegistryStatus(t *testing.T) {
	// given a session that hooks left in working (e.g. turn interrupted with Esc)
	s := newStore()
	s.Apply(ev(event.UserPromptSubmit, 0))

	// when the registry says idle later
	changed := s.Sync([]Discovered{disc("idle", time.Minute)})

	// then
	if got := only(t, s); !changed || got.State != Ready || !got.Since.Equal(t0.Add(time.Minute)) {
		t.Fatalf("changed=%v %+v", changed, got)
	}

	// and busy again later → working
	s.Sync([]Discovered{disc("busy", 2*time.Minute)})
	if got := only(t, s); got.State != Working {
		t.Fatalf("%+v", got)
	}
}

func TestShouldIgnoreOlderRegistryStatus(t *testing.T) {
	// given hooks are more recent than the registry
	s := newStore()
	s.Apply(ev(event.UserPromptSubmit, time.Minute))

	// when
	s.Sync([]Discovered{disc("idle", 0)})

	// then
	if got := only(t, s); got.State != Working {
		t.Fatalf("%+v", got)
	}
}

func TestShouldNeverClearInputFromRegistry(t *testing.T) {
	s := newStore()
	s.Apply(ev(event.PermissionRequest, 0))
	s.Sync([]Discovered{disc("idle", time.Minute)})
	s.Sync([]Discovered{disc("busy", 2*time.Minute)})
	if got := only(t, s); got.State != Input {
		t.Fatalf("%+v", got)
	}
}

func TestShouldWakeIdleSessionWhenBusy(t *testing.T) {
	// given
	s := newStore()
	s.Apply(ev(event.Stop, 0))
	s.Tick(t0.Add(20*time.Minute), func(int) bool { return true })

	// when
	s.Sync([]Discovered{disc("busy", 21*time.Minute)})

	// then
	if got := only(t, s); got.State != Working {
		t.Fatalf("%+v", got)
	}
}

func TestShouldFillMissingPidFromRegistry(t *testing.T) {
	s := newStore()
	e := ev(event.Stop, time.Minute)
	e.PID = 0
	s.Apply(e)
	s.Sync([]Discovered{disc("idle", 0)})
	if got := only(t, s); got.PID != 20169 {
		t.Fatalf("%+v", got)
	}
}

func TestShouldUseRegistryName(t *testing.T) {
	// given
	s := newStore()
	d := disc("idle", 0)
	d.Name = "field-proof-75"

	// when
	s.Sync([]Discovered{d})

	// then
	if got := only(t, s); got.Name != "field-proof-75" {
		t.Fatalf("%+v", got)
	}
}

func TestShouldRenameHookSessionsAndFollowRenames(t *testing.T) {
	// given a session created by a hook, named after its folder
	s := newStore()
	s.Apply(ev(event.UserPromptSubmit, time.Minute))
	d := disc("busy", 0)
	d.Name = "cyberviber-fa"

	// when
	changed := s.Sync([]Discovered{d})

	// then
	if got := only(t, s); !changed || got.Name != "cyberviber-fa" {
		t.Fatalf("changed=%v %+v", changed, got)
	}

	// and a later /rename is picked up
	d.Name = "daemon work"
	s.Sync([]Discovered{d})
	if got := only(t, s); got.Name != "daemon work" {
		t.Fatalf("%+v", got)
	}
}

func TestShouldKeepFolderNameWhenRegistryHasNone(t *testing.T) {
	s := newStore()
	s.Sync([]Discovered{disc("idle", 0)})
	if got := only(t, s); got.Name != "field-proof" {
		t.Fatalf("%+v", got)
	}
}
