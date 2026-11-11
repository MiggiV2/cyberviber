package focus

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/d135-1r43/cyberviber/internal/session"
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

type tabby struct{}

// Focus asks the plugin for the tab and always activates Tabby, even when another app is frontmost.
func (tabby) Focus(f *Focuser, _ session.Session, t Terminal, _ bool) (int, string) {
	tab, err := FocusTabby(f.TabbyDir, t.Chain)
	f.Run("open", "-a", t.Bundle)
	switch {
	case errors.Is(err, ErrNoPlugin):
		return 0, "no-plugin"
	case err != nil:
		return 0, "tab-not-found"
	}
	return tab, ""
}
