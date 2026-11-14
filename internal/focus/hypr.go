package focus

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/d135-1r43/cyberviber/internal/session"
)

// hyprWindow focuses the window of a Linux terminal that has no scripting interface of its own
// (foot, alacritty, Ghostty on Linux, …) through Hyprland. A terminal process owning one window
// is focused by that window's address. Several windows on one process (foot --server, Ghostty,
// alacritty msg) are told apart by writing a unique title marker to the session's outer TTY.
type hyprWindow struct{}

type hyprClient struct {
	Address string `json:"address"`
	PID     int    `json:"pid"`
	Title   string `json:"title"`
}

func (hyprWindow) Focus(f *Focuser, _ session.Session, t Terminal, _ bool) (int, string) {
	wins, ok := f.hyprWindows(t.AppPID)
	if !ok {
		f.activate(t) // not Hyprland
		return 0, "tab-not-found"
	}
	if len(wins) == 1 {
		f.hyprFocus("address:" + wins[0].Address)
		return 0, ""
	}
	if len(wins) > 1 && t.TTY != "" {
		marker := "cv-" + randomHex(6)
		if writeTitle(t.TTY, marker) == nil {
			for i := 0; i < 6; i++ {
				time.Sleep(50 * time.Millisecond)
				wins, _ = f.hyprWindows(t.AppPID)
				for _, w := range wins {
					if strings.Contains(w.Title, marker) {
						f.hyprFocus("address:" + w.Address)
						return 0, ""
					}
				}
			}
		}
	}
	f.activate(t)
	return 0, "tab-not-found"
}

// hyprWindows lists the Hyprland windows of a process; ok is false when hyprctl does not answer.
func (f *Focuser) hyprWindows(pid int) (wins []hyprClient, ok bool) {
	out, err := f.Run("hyprctl", "-i", "0", "-j", "clients")
	if err != nil {
		return nil, false
	}
	var all []hyprClient
	if json.Unmarshal(out, &all) != nil {
		return nil, false
	}
	for _, c := range all {
		if c.PID == pid {
			wins = append(wins, c)
		}
	}
	return wins, true
}

// hyprFocus focuses a window (Hyprland selector, e.g. pid:123 or address:0x…), which also switches
// to its workspace.
func (f *Focuser) hyprFocus(window string) {
	out, err := f.Run("hyprctl", "-i", "0", "dispatch", `hl.dsp.focus({ window = "`+window+`" })`)
	if err == nil && strings.TrimSpace(string(out)) == "ok" {
		return
	}
	// Hyprland before 0.57 only knows the classic dispatcher syntax
	f.Run("hyprctl", "-i", "0", "dispatch", "focuswindow", window)
}
