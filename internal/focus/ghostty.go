package focus

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/d135-1r43/cyberviber/internal/session"
)

// Ghostty (≥ 1.3) is scripted via AppleScript. Its terminals expose id, name and working directory
// but no TTY, so the terminal is found by cached id, then by a unique working directory, then by
// writing a unique title marker to the outer TTY and looking for it.

func asString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func ghosttyFocusByID(id string) string {
	return fmt.Sprintf(`tell application "Ghostty"
	set ts to every terminal whose id is %s
	if (count of ts) is 0 then return ""
	focus item 1 of ts
	activate
	return "ok"
end tell`, asString(id))
}

func ghosttyFocusByCwd(cwd string) string {
	return fmt.Sprintf(`tell application "Ghostty"
	set ts to every terminal whose working directory is %s
	if (count of ts) is not 1 then return ""
	focus item 1 of ts
	activate
	return id of item 1 of ts
end tell`, asString(cwd))
}

func ghosttyFocusByName(marker string) string {
	return fmt.Sprintf(`tell application "Ghostty"
	set ts to every terminal whose name contains %s
	if (count of ts) is 0 then return ""
	focus item 1 of ts
	activate
	return id of item 1 of ts
end tell`, asString(marker))
}

func (f *Focuser) focusGhostty(sessionID, cwd, tty string, inZellij bool) bool {
	osa := func(script string) string {
		out, err := f.Run("osascript", "-e", script)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	f.mu.Lock()
	cached := f.ghosttyIDs[sessionID]
	f.mu.Unlock()
	remember := func(id string) bool {
		f.mu.Lock()
		if f.ghosttyIDs == nil {
			f.ghosttyIDs = map[string]string{}
		}
		f.ghosttyIDs[sessionID] = id
		f.mu.Unlock()
		return true
	}

	if cached != "" && osa(ghosttyFocusByID(cached)) == "ok" {
		return true
	}
	if !inZellij && cwd != "" {
		if id := osa(ghosttyFocusByCwd(cwd)); id != "" {
			return remember(id)
		}
	}
	if tty == "" {
		return false
	}
	marker := "cv-" + randomHex(6)
	if err := writeTitle(tty, marker); err != nil {
		return false
	}
	for i := 0; i < 6; i++ {
		time.Sleep(50 * time.Millisecond)
		if id := osa(ghosttyFocusByName(marker)); id != "" {
			return remember(id)
		}
	}
	return false
}

// writeTitle sets the outer terminal's title via OSC 2. Claude Code / zellij overwrite it again later.
var writeTitle = func(tty, title string) error {
	f, err := os.OpenFile("/dev/"+tty, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\x1b]2;%s\x07", title)
	return err
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type ghostty struct{}

func (ghostty) Focus(f *Focuser, s session.Session, t Terminal, inMux bool) (int, string) {
	if f.goos() == "darwin" && f.focusGhostty(s.ID, s.Cwd, t.TTY, inMux) {
		return 0, ""
	}
	f.activate(t)
	return 0, "tab-not-found"
}
