package focus

import (
	"bufio"
	"strconv"
	"strings"

	"github.com/d135-1r43/cyberviber/internal/session"
)

type kitty struct{}

// Focus uses kitty's remote control (`allow_remote_control` + `listen_on unix:…` in kitty.conf).
// The socket path gets kitty's PID appended, so it is read from the running kitty with lsof.
// `pid:` matches a window's child process, which is the tab root of the chain.
func (kitty) Focus(f *Focuser, _ session.Session, t Terminal, _ bool) (int, string) {
	defer f.Run("open", "-a", t.Bundle)
	socks := kittySockets(f, t.AppPID)
	if len(socks) == 0 {
		return 0, "no-remote-control"
	}
	match := make([]string, len(t.Chain))
	for i, pid := range t.Chain {
		match[i] = "pid:" + strconv.Itoa(pid)
	}
	kitten := t.Bundle + "/Contents/MacOS/kitten"
	for _, sock := range socks {
		if _, err := f.Run(kitten, "@", "--to", "unix:"+sock, "focus-window", "--match", strings.Join(match, " or ")); err == nil {
			return 0, ""
		}
	}
	return 0, "tab-not-found"
}

func kittySockets(f *Focuser, pid int) []string {
	out, _ := f.Run("lsof", "-a", "-p", strconv.Itoa(pid), "-U", "-F", "n")
	var socks []string
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		if path, ok := strings.CutPrefix(sc.Text(), "n/"); ok {
			socks = append(socks, "/"+path)
		}
	}
	return socks
}
