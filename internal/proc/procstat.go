package proc

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
)

// parseStat reads comm, ppid and tty_nr from /proc/<pid>/stat. comm may contain spaces and
// parentheses, so it ends at the last ')'.
func parseStat(stat string) (comm string, ppid, ttyNr int, err error) {
	open, end := strings.IndexByte(stat, '('), strings.LastIndexByte(stat, ')')
	if open < 0 || end < open {
		return "", 0, 0, errors.New("malformed stat")
	}
	f := strings.Fields(stat[end+1:])
	if len(f) < 5 {
		return "", 0, 0, errors.New("malformed stat")
	}
	ppid, err = strconv.Atoi(f[1])
	if err != nil {
		return "", 0, 0, err
	}
	ttyNr, err = strconv.Atoi(f[4])
	return stat[open+1 : end], ppid, ttyNr, err
}

// ttyFromNr names the device of a Linux tty_nr (major in bits 8–19, minor split around it).
func ttyFromNr(nr int) string {
	major := (nr >> 8) & 0xfff
	minor := (nr & 0xff) | ((nr >> 12) & 0xfff00)
	switch {
	case major >= 136 && major <= 143:
		return "pts/" + strconv.Itoa((major-136)<<8+minor)
	case major == 4 && minor < 64 && nr != 0:
		return "tty" + strconv.Itoa(minor)
	}
	return ""
}

func parseEnviron(b []byte) map[string]string {
	m := map[string]string{}
	for _, kv := range bytes.Split(b, []byte{0}) {
		if k, v, ok := strings.Cut(string(kv), "="); ok {
			m[k] = v
		}
	}
	return m
}

// exeLink cleans a /proc/<pid>/exe target: after a package upgrade replaced the binary, the
// kernel appends " (deleted)".
func exeLink(target string) string {
	return strings.TrimSuffix(target, " (deleted)")
}
