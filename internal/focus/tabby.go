package focus

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"time"
)

var (
	ErrNoPlugin = errors.New("tabby plugin not running")
	ErrNotFound = errors.New("tab not found")
)

type focusRequest struct {
	Cmd  string `json:"cmd"`
	PIDs []int  `json:"pids"`
}

type focusResponse struct {
	OK    bool   `json:"ok"`
	Index int    `json:"index"`
	Error string `json:"error"`
}

// TabbyDir is where the tabby-cyberviber plugin listens, one socket per Tabby window.
func TabbyDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cyberviber", "tabby")
}

// FocusTabby asks every Tabby window's plugin to focus the tab whose shell PID is one of pids.
// It returns the 1-based tab index. Sockets nobody listens on are removed.
func FocusTabby(dir string, pids []int) (int, error) {
	socks, _ := filepath.Glob(filepath.Join(dir, "*.sock"))
	reached := false
	for _, sock := range socks {
		c, err := net.DialTimeout("unix", sock, 500*time.Millisecond)
		if err != nil {
			os.Remove(sock) // window closed without cleanup
			continue
		}
		reached = true
		c.SetDeadline(time.Now().Add(2 * time.Second))
		var res focusResponse
		err = json.NewEncoder(c).Encode(focusRequest{Cmd: "focus", PIDs: pids})
		if err == nil {
			err = json.NewDecoder(bufio.NewReader(c)).Decode(&res)
		}
		c.Close()
		if err == nil && res.OK {
			return res.Index, nil
		}
	}
	if !reached {
		return 0, ErrNoPlugin
	}
	return 0, ErrNotFound
}
