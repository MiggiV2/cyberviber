package focus

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ParseLsof maps zellij session names to the PIDs of their attached clients, from the output of
// `lsof -U -a -c zellij -F pfdn`. The server's socket fds carry the session socket path; a client's
// connection points ("->0x…") at the address of one of those fds.
func ParseLsof(out string) map[string][]int {
	type fd struct{ dev, name string }
	fds := map[int][]fd{}
	pid := 0
	var cur fd
	flush := func() {
		if pid != 0 && (cur.dev != "" || cur.name != "") {
			fds[pid] = append(fds[pid], cur)
		}
		cur = fd{}
	}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		v := line[1:]
		switch line[0] {
		case 'p':
			flush()
			pid, _ = strconv.Atoi(v)
		case 'f':
			flush()
		case 'd':
			cur.dev = v
		case 'n':
			cur.name = v
		}
	}
	flush()

	sessionOf := map[string]string{} // socket address → session name
	for _, list := range fds {
		for _, f := range list {
			if strings.HasPrefix(f.name, "/") && strings.Contains(f.name, "/zellij-") {
				sessionOf[f.dev] = filepath.Base(f.name)
			}
		}
	}
	clients := map[string][]int{}
	for p, list := range fds {
		for _, f := range list {
			if peer, ok := strings.CutPrefix(f.name, "->"); ok {
				if s, ok := sessionOf[peer]; ok {
					clients[s] = append(clients[s], p)
					break
				}
			}
		}
	}
	return clients
}

func paneID(id string) string {
	if strings.HasPrefix(id, "terminal_") || strings.HasPrefix(id, "plugin_") {
		return id
	}
	return "terminal_" + id
}

// lookZellij finds the zellij binary; launchd agents run with a minimal PATH.
func lookZellij() string {
	if p, err := exec.LookPath("zellij"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{"/opt/homebrew/bin/zellij", "/usr/local/bin/zellij",
		filepath.Join(home, ".cargo", "bin", "zellij"), filepath.Join(home, ".local", "bin", "zellij")} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "zellij"
}
