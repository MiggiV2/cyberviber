package proc

import (
	"errors"
	"os"
	"testing"
)

func TestShouldReadOwnProcess(t *testing.T) {
	// when
	p, err := Get(os.Getpid())

	// then
	if err != nil {
		t.Fatal(err)
	}
	if p.PID != os.Getpid() || p.PPID != os.Getppid() || p.Comm == "" {
		t.Fatalf("unexpected %+v", p)
	}
}

func TestShouldDetectAliveProcesses(t *testing.T) {
	if !Alive(os.Getpid()) {
		t.Fatal("own process must be alive")
	}
	if Alive(999999) {
		t.Fatal("pid 999999 must not be alive")
	}
}

func fakeTable(procs ...Proc) func(int) (Proc, error) {
	m := map[int]Proc{}
	for _, p := range procs {
		m[p.PID] = p
	}
	return func(pid int) (Proc, error) {
		if p, ok := m[pid]; ok {
			return p, nil
		}
		return Proc{}, errors.New("no such process")
	}
}

func TestShouldFindClaudeAboveHookShell(t *testing.T) {
	// given hook(100) <- sh(90) <- claude(80) <- zsh(70) <- zellij(1)
	get := fakeTable(
		Proc{PID: 90, PPID: 80, Comm: "sh"},
		Proc{PID: 80, PPID: 70, Comm: "claude", TTY: "ttys022"},
		Proc{PID: 70, PPID: 1, Comm: "zsh"},
	)

	// when
	p, ok := findAgent(90, get)

	// then
	if !ok || p.PID != 80 || p.TTY != "ttys022" {
		t.Fatalf("got %+v ok=%v", p, ok)
	}
}

func TestShouldFallBackToFirstNonShellAncestor(t *testing.T) {
	// given claude installed via npm runs as node
	get := fakeTable(
		Proc{PID: 90, PPID: 85, Comm: "bash"},
		Proc{PID: 85, PPID: 80, Comm: "zsh"},
		Proc{PID: 80, PPID: 70, Comm: "node"},
		Proc{PID: 70, PPID: 1, Comm: "zsh"},
	)

	// when
	p, ok := findAgent(90, get)

	// then
	if !ok || p.PID != 80 {
		t.Fatalf("got %+v ok=%v", p, ok)
	}
}

func TestShouldGiveUpAtLaunchd(t *testing.T) {
	get := fakeTable(Proc{PID: 90, PPID: 1, Comm: "sh"})
	if _, ok := findAgent(90, get); ok {
		t.Fatal("expected no agent")
	}
}
