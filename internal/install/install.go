// Package install adds/removes the cyberviber hooks in Claude Code's settings.json and renders the launchd agent.
package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Marker identifies our hook commands so they can be updated and removed without touching others.
const Marker = "# cyberviber"

const Label = "dev.cyberviber.daemon"

var Events = []string{
	"SessionStart", "SessionEnd", "UserPromptSubmit", "PreToolUse", "PostToolUse",
	"PostToolUseFailure", "PermissionRequest", "Notification", "Stop",
}

func command(bin, sub string) string {
	return fmt.Sprintf("'%s' %s %s", strings.ReplaceAll(bin, "'", `'\''`), sub, Marker)
}

// ours reports whether a status line setting is cyberviber's.
func ours(v any) bool {
	m, _ := v.(map[string]any)
	c, _ := m["command"].(string)
	return strings.Contains(c, Marker)
}

// StatusLineTaken reports whether settings has a status line that is not cyberviber's;
// then install leaves it alone and context usage stays unknown.
func StatusLineTaken(settings []byte) bool {
	root, _, err := parse(settings)
	if err != nil {
		return false
	}
	v, set := root["statusLine"]
	return set && !ours(v)
}

// MergeHooks returns settings with one cyberviber hook per event and cyberviber's status line. Existing cyberviber hooks are
// replaced, all other settings and hooks are kept. Note: keys are re-sorted by encoding/json.
func MergeHooks(settings []byte, bin string) ([]byte, error) {
	root, hooks, err := parse(settings)
	if err != nil {
		return nil, err
	}
	for _, ev := range Events {
		groups := withoutOurs(hooks[ev])
		groups = append(groups, map[string]any{
			"hooks": []any{map[string]any{"type": "command", "command": command(bin, "hook"), "timeout": 5}},
		})
		hooks[ev] = groups
	}
	root["hooks"] = hooks
	// the status line reports context usage; a foreign one is kept
	if v, set := root["statusLine"]; !set || ours(v) {
		root["statusLine"] = map[string]any{"type": "command", "command": command(bin, "statusline")}
	}
	return encode(root)
}

// RemoveHooks removes all cyberviber hooks (dropping events that end up empty) and its status line.
func RemoveHooks(settings []byte) ([]byte, error) {
	root, hooks, err := parse(settings)
	if err != nil {
		return nil, err
	}
	for ev, g := range hooks {
		groups := withoutOurs(g)
		if len(groups) == 0 {
			delete(hooks, ev)
		} else {
			hooks[ev] = groups
		}
	}
	if v, set := root["statusLine"]; set && ours(v) {
		delete(root, "statusLine")
	}
	if len(hooks) == 0 {
		delete(root, "hooks")
	} else {
		root["hooks"] = hooks
	}
	return encode(root)
}

func parse(settings []byte) (map[string]any, map[string]any, error) {
	root := map[string]any{}
	if len(bytes.TrimSpace(settings)) > 0 {
		if err := json.Unmarshal(settings, &root); err != nil {
			return nil, nil, fmt.Errorf("settings.json: %w", err)
		}
	}
	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	return root, hooks, nil
}

func withoutOurs(v any) []any {
	groups, _ := v.([]any)
	var out []any
	for _, g := range groups {
		gm, ok := g.(map[string]any)
		if !ok {
			out = append(out, g)
			continue
		}
		hs, _ := gm["hooks"].([]any)
		var keep []any
		for _, h := range hs {
			if hm, ok := h.(map[string]any); ok {
				if c, _ := hm["command"].(string); strings.Contains(c, Marker) {
					continue
				}
			}
			keep = append(keep, h)
		}
		if len(keep) == 0 && len(hs) > 0 {
			continue
		}
		gm["hooks"] = keep
		out = append(out, gm)
	}
	return out
}

func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// LaunchAgentPlist renders the launchd agent that keeps `cyberviber daemon` running.
func LaunchAgentPlist(bin, logPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>daemon</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, Label, xmlEscape(bin), xmlEscape(logPath), xmlEscape(logPath))
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
