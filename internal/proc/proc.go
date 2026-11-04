// Package proc reads the macOS process table via sysctl (no cgo, no ps).
package proc

import (
	"errors"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

type Proc struct {
	PID  int
	PPID int
	Comm string
	TTY  string // e.g. "ttys022", "" when the process has no controlling terminal
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
	return Proc{
		PID:  pid,
		PPID: int(kp.Eproc.Ppid),
		Comm: unix.ByteSliceToString(kp.Proc.P_comm[:]),
		TTY:  ttyName(kp.Eproc.Tdev),
	}, nil
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
		if strings.EqualFold(p.Comm, "claude") {
			return p, true
		}
		if fallback == nil && !shells[strings.TrimPrefix(p.Comm, "-")] {
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
