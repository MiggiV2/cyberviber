package proc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

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

// args reads the executable path and argv[0] via kern.procargs2.
func args(pid int) (exe, name string) {
	b, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return "", ""
	}
	exe, argv, _ := parseProcargs(b)
	if len(argv) > 0 {
		name = filepath.Base(argv[0])
	}
	return exe, name
}

// Env returns the environment of a process (readable for processes of the same user).
func Env(pid int) map[string]string {
	b, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return nil
	}
	_, _, env := parseProcargs(b)
	m := make(map[string]string, len(env))
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			m[k] = v
		}
	}
	return m
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
