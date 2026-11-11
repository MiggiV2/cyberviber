package focus

import (
	"log"
	"path/filepath"
	"strings"

	"github.com/d135-1r43/cyberviber/internal/mux"
)

type zellij struct{}

// Focus runs `zellij action focus-pane-id`, which also switches to the pane's tab. It uses the
// attached client's own binary (same version); launchd agents have a minimal PATH.
func (zellij) Focus(f *Focuser, loc mux.Location) []int {
	clients := f.socketClients("zellij", func(path string) bool {
		return strings.Contains(path, "/zellij") && filepath.Base(path) == loc.Session
	})
	bin := ""
	if len(clients) > 0 {
		if c, err := f.Get(clients[0]); err == nil {
			bin = c.Exe
		}
	}
	if bin == "" {
		bin = lookBinary("zellij")
	}
	if loc.Pane != "" {
		if out, err := f.Run(bin, "--session", loc.Session, "action", "focus-pane-id", paneID(loc.Pane)); err != nil {
			log.Printf("focus: zellij focus-pane-id: %v %s", err, out)
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
