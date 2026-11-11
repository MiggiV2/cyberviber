package focus

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/d135-1r43/cyberviber/internal/session"
)

type kitty struct{}

// Focus uses kitty's remote control (`allow_remote_control` + `listen_on unix:…` in kitty.conf).
// kitty exports the socket as KITTY_LISTEN_ON to the processes in its windows; `pid:` matches a
// window's child process, which is the tab root of the chain.
func (kitty) Focus(f *Focuser, _ session.Session, t Terminal, _ bool) (int, string) {
	defer f.activate(t)
	sock := ""
	for _, pid := range t.Chain {
		if sock = f.Env(pid)["KITTY_LISTEN_ON"]; sock != "" {
			break
		}
	}
	if sock == "" {
		return 0, "no-remote-control"
	}
	match := make([]string, len(t.Chain))
	for i, pid := range t.Chain {
		match[i] = "pid:" + strconv.Itoa(pid)
	}
	if _, err := f.Run(kitten(t), "@", "--to", sock, "focus-window", "--match", strings.Join(match, " or ")); err != nil {
		return 0, "tab-not-found"
	}
	return 0, ""
}

func kitten(t Terminal) string {
	if strings.HasSuffix(t.Bundle, ".app") {
		return t.Bundle + "/Contents/MacOS/kitten"
	}
	return filepath.Join(filepath.Dir(t.Bundle), "kitten")
}
