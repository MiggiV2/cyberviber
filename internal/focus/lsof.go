package focus

import (
	"bufio"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// ParseLsof maps unix socket paths to the PIDs of the processes connected to them, from the output
// of `lsof -U -F pfdn`. A server's socket fds carry the path; a client's connection points
// ("->0x…") at the address of one of those fds.
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

	pathOf := map[string]string{} // socket address → path
	for _, list := range fds {
		for _, f := range list {
			if strings.HasPrefix(f.name, "/") {
				pathOf[f.dev] = f.name
			}
		}
	}
	clients := map[string][]int{}
	for p, list := range fds {
		for _, f := range list {
			peer, ok := strings.CutPrefix(f.name, "->")
			if !ok {
				continue
			}
			if path, ok := pathOf[peer]; ok && !slices.Contains(clients[path], p) {
				clients[path] = append(clients[path], p)
			}
		}
	}
	return clients
}

// socketClients lists the processes named command that are connected to a unix socket whose path
// matches, lowest PID first.
func (f *Focuser) socketClients(command string, match func(path string) bool) []int {
	out, err := f.Run("lsof", "-U", "-a", "-c", command, "-F", "pfdn")
	if err != nil && len(out) == 0 {
		log.Printf("focus: lsof -c %s: %v", command, err)
	}
	var pids []int
	for path, ps := range ParseLsof(string(out)) {
		if match(path) {
			pids = append(pids, ps...)
		}
	}
	slices.Sort(pids)
	return slices.Compact(pids)
}

// lookBinary finds a binary; launchd agents run with a minimal PATH.
func lookBinary(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin",
		filepath.Join(home, ".cargo", "bin"), filepath.Join(home, ".local", "bin")} {
		if p := filepath.Join(dir, name); fileExists(p) {
			return p
		}
	}
	return name
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
