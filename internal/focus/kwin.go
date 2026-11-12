package focus

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const kwinScriptName = "cyberviber-focus"

// Plasma 6 names the list windowList/activeWindow, Plasma 5 clientList/activeClient. A terminal
// with several windows shares one PID; kitty asks for attention on the window it just focused.
const kwinScript = `const pid = %d;
const six = typeof workspace.windowList === "function";
const all = (six ? workspace.windowList() : workspace.clientList()).filter(w => w.pid === pid);
const w = all.find(w => w.demandsAttention) || all[0];
if (w) {
  w.minimized = false;
  if (six) workspace.activeWindow = w; else workspace.activeClient = w;
}
`

var dbusInt32 = regexp.MustCompile(`int32 (-?\d+)`)

func plasma(env map[string]string) bool {
	return strings.Contains(strings.ToUpper(env["XDG_CURRENT_DESKTOP"]), "KDE")
}

func (f *Focuser) kwinCall(path, method string, args ...string) ([]byte, error) {
	return f.Run("dbus-send", append([]string{"--session", "--print-reply", "--dest=org.kde.KWin", path, method}, args...)...)
}

// activateKWin raises the terminal through a one-shot KWin script: under Wayland only the
// compositor may raise a window, and KWin has no plain D-Bus call for "activate the window of PID".
func (f *Focuser) activateKWin(pid int) {
	dir := f.ScriptDir
	if dir == "" {
		dir = os.TempDir()
	}
	path := filepath.Join(dir, "cyberviber-kwin-focus.js")
	if os.WriteFile(path, []byte(fmt.Sprintf(kwinScript, pid)), 0o600) != nil {
		return
	}
	f.kwinCall("/Scripting", "org.kde.kwin.Scripting.unloadScript", "string:"+kwinScriptName)
	out, err := f.kwinCall("/Scripting", "org.kde.kwin.Scripting.loadScript", "string:"+path, "string:"+kwinScriptName)
	m := dbusInt32.FindSubmatch(out)
	if err != nil || m == nil {
		return
	}
	id, _ := strconv.Atoi(string(m[1]))
	if id < 0 {
		return
	}
	if _, err := f.kwinCall(fmt.Sprintf("/Scripting/Script%d", id), "org.kde.kwin.Script.run"); err != nil {
		f.kwinCall(fmt.Sprintf("/%d", id), "org.kde.kwin.Script.run")
	}
}
