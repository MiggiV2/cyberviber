// Package proc reads the macOS process table via sysctl (no cgo, no ps).
package proc

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

type Proc struct {
	PID  int
	PPID int
	Comm string // kernel name; for Claude Code this is its version, e.g. "2.1.316"
	Name string // basename of argv[0], e.g. "claude", "-zsh"; falls back to Comm
	Exe  string // executable path from the kernel, if readable
	TTY  string // e.g. "ttys022", "" when the process has no controlling terminal
}

// IsClaude reports whether the process is the Claude Code CLI.
func (p Proc) IsClaude() bool {
	return strings.EqualFold(p.Name, "claude") || strings.EqualFold(p.Comm, "claude") ||
		strings.Contains(p.Exe, "/claude/versions/")
}

// Get reads one process.
func Get(pid int) (Proc, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return Proc{}, err
	}
	if int(kp.Proc.P_pid) != pid {
		return Proc{}, errors.New("no such process")
	}
	p := Proc{
		PID:  pid,
		PPID: int(kp.Eproc.Ppid),
		Comm: unix.ByteSliceToString(kp.Proc.P_comm[:]),
		TTY:  ttyName(kp.Eproc.Tdev),
	}
	p.Exe, p.Name = args(pid)
	if p.Name == "" {
		p.Name = p.Comm
	}
	return p, nil
}

// args reads the executable path and argv[0] via kern.procargs2:
// int32 argc, exec path, NUL padding, argv[0], …
func args(pid int) (exe, name string) {
	b, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil || len(b) < 4 {
		return "", ""
	}
	rest := b[4:]
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return "", ""
	}
	exe, rest = string(rest[:i]), rest[i:]
	rest = bytes.TrimLeft(rest, "\x00")
	if j := bytes.IndexByte(rest, 0); j >= 0 {
		rest = rest[:j]
	}
	return exe, filepath.Base(string(rest))
}

// Alive reports whether a process with this PID exists.
func Alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// FindAgent walks up from pid (usually the hook's parent) to the Claude Code process.
func FindAgent(pid int) (Proc, bool) { return findAgent(pid, Get) }

var shells = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "fish": true, "nu": true}

func findAgent(pid int, get func(int) (Proc, error)) (Proc, bool) {
	var fallback *Proc
	for i := 0; i < 32 && pid > 1; i++ {
		p, err := get(pid)
		if err != nil {
			break
		}
		if p.IsClaude() {
			return p, true
		}
		name := p.Name
		if name == "" {
			name = p.Comm
		}
		if fallback == nil && !shells[strings.TrimPrefix(name, "-")] {
			fp := p
			fallback = &fp
		}
		pid = p.PPID
	}
	if fallback != nil {
		return *fallback, true
	}
	return Proc{}, false
}

func ttyName(dev int32) string {
	if dev == -1 || dev == 0 {
		return ""
	}
	entries, err := os.ReadDir("/dev")
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "tty") {
			continue
		}
		var st unix.Stat_t
		if unix.Stat("/dev/"+e.Name(), &st) == nil && int32(st.Rdev) == dev {
			return e.Name()
		}
	}
	return ""
}
