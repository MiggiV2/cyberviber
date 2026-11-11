package proc

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Get reads one process from /proc.
func Get(pid int) (Proc, error) {
	dir := "/proc/" + strconv.Itoa(pid)
	stat, err := os.ReadFile(dir + "/stat")
	if err != nil {
		return Proc{}, err
	}
	comm, ppid, ttyNr, err := parseStat(string(stat))
	if err != nil {
		return Proc{}, err
	}
	p := Proc{PID: pid, PPID: ppid, Comm: comm, TTY: ttyFromNr(ttyNr)}
	if exe, err := os.Readlink(dir + "/exe"); err == nil {
		p.Exe = exeLink(exe)
	}
	if cmdline, err := os.ReadFile(dir + "/cmdline"); err == nil {
		argv0, _, _ := bytes.Cut(cmdline, []byte{0})
		if f := strings.Fields(string(argv0)); len(f) > 0 {
			p.Name = filepath.Base(f[0])
		}
	}
	if p.Name == "" {
		p.Name = p.Comm
	}
	return p, nil
}

// Env returns the environment of a process (readable for processes of the same user).
func Env(pid int) map[string]string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/environ")
	if err != nil {
		return nil
	}
	return parseEnviron(b)
}
