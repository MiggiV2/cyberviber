package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const existing = `{
  "model": "opus",
  "hooks": {
    "Stop": [
      { "hooks": [ { "type": "command", "command": "notchbar.sh Stop" } ] }
    ]
  }
}`

func hooksOf(t *testing.T, b []byte) map[string][]any {
	t.Helper()
	var s struct {
		Hooks map[string][]any `json:"hooks"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s.Hooks
}

func TestShouldAddHooksForAllEventsAndKeepForeignOnes(t *testing.T) {
	// when
	out, err := MergeHooks([]byte(existing), "/usr/local/bin/cyberviber")

	// then
	if err != nil {
		t.Fatal(err)
	}
	h := hooksOf(t, out)
	for _, e := range Events {
		if len(h[e]) == 0 {
			t.Errorf("missing hook for %s", e)
		}
	}
	if len(h["Stop"]) != 2 {
		t.Fatalf("Stop must keep the foreign hook and add ours: %v", h["Stop"])
	}
	if !strings.Contains(string(out), `"model": "opus"`) {
		t.Fatal("other settings must be kept")
	}
	if !strings.Contains(string(out), `'/usr/local/bin/cyberviber' hook `+Marker) {
		t.Fatalf("command missing:\n%s", out)
	}
}

func TestShouldBeIdempotent(t *testing.T) {
	// given
	once, _ := MergeHooks([]byte(existing), "/bin/cv")

	// when
	twice, err := MergeHooks(once, "/bin/cv")

	// then
	if err != nil {
		t.Fatal(err)
	}
	if string(once) != string(twice) {
		t.Fatalf("not idempotent:\n%s\n---\n%s", once, twice)
	}
}

func TestShouldReplaceOldBinaryPath(t *testing.T) {
	once, _ := MergeHooks([]byte(existing), "/old/cv")
	out, _ := MergeHooks(once, "/new/cv")
	if strings.Contains(string(out), "/old/cv") || len(hooksOf(t, out)["Stop"]) != 2 {
		t.Fatalf("old path must be replaced:\n%s", out)
	}
}

func TestShouldRemoveOnlyOwnHooks(t *testing.T) {
	// given
	in, _ := MergeHooks([]byte(existing), "/bin/cv")

	// when
	out, err := RemoveHooks(in)

	// then
	if err != nil {
		t.Fatal(err)
	}
	h := hooksOf(t, out)
	if strings.Contains(string(out), Marker) {
		t.Fatalf("own hooks left:\n%s", out)
	}
	if len(h["Stop"]) != 1 || len(h) != 1 {
		t.Fatalf("foreign hooks must stay, empty events removed: %v", h)
	}
}

func TestShouldHandleEmptySettings(t *testing.T) {
	out, err := MergeHooks(nil, "/bin/cv")
	if err != nil || len(hooksOf(t, out)) != len(Events) {
		t.Fatalf("err=%v out=%s", err, out)
	}
}

func TestShouldRenderLaunchAgent(t *testing.T) {
	p := LaunchAgentPlist("/bin/cv", "/tmp/cv.log")
	for _, want := range []string{"<string>/bin/cv</string>", "<string>daemon</string>", Label, "<key>KeepAlive</key>"} {
		if !strings.Contains(p, want) {
			t.Errorf("plist missing %q", want)
		}
	}
}

func TestShouldInstallAndRemoveTabbyPlugin(t *testing.T) {
	// given
	dir := t.TempDir()

	// when
	path, err := InstallTabbyPlugin(dir)

	// then
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"package.json", "index.js"} {
		if _, err := os.Stat(filepath.Join(path, f)); err != nil {
			t.Fatalf("%s missing: %v", f, err)
		}
	}
	if filepath.Base(path) != "tabby-cyberviber" || filepath.Base(filepath.Dir(path)) != "node_modules" {
		t.Fatalf("unexpected path %s", path)
	}

	// when
	if err := RemoveTabbyPlugin(dir); err != nil {
		t.Fatal(err)
	}

	// then
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("plugin must be removed")
	}
}

func statusLineOf(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var s struct {
		StatusLine map[string]any `json:"statusLine"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s.StatusLine
}

func TestShouldInstallStatusLineWhenNoneIsSet(t *testing.T) {
	out, err := MergeHooks([]byte(existing), "/bin/cv")
	if err != nil {
		t.Fatal(err)
	}
	sl := statusLineOf(t, out)
	if sl["type"] != "command" || sl["command"] != "'/bin/cv' statusline "+Marker {
		t.Fatalf("statusLine = %v", sl)
	}
	if StatusLineTaken(out) {
		t.Fatal("our own status line must not count as taken")
	}
}

func TestShouldKeepForeignStatusLine(t *testing.T) {
	in := `{"statusLine": {"type": "command", "command": "~/.claude/mine.sh"}}`
	out, err := MergeHooks([]byte(in), "/bin/cv")
	if err != nil {
		t.Fatal(err)
	}
	if sl := statusLineOf(t, out); sl["command"] != "~/.claude/mine.sh" {
		t.Fatalf("foreign status line replaced: %v", sl)
	}
	if !StatusLineTaken(out) {
		t.Fatal("foreign status line must count as taken")
	}
	out, _ = RemoveHooks(out)
	if sl := statusLineOf(t, out); sl["command"] != "~/.claude/mine.sh" {
		t.Fatalf("uninstall removed the foreign status line: %v", sl)
	}
}

func TestShouldRemoveOwnStatusLine(t *testing.T) {
	in, _ := MergeHooks([]byte(existing), "/bin/cv")
	out, err := RemoveHooks(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "statusLine") {
		t.Fatalf("status line left behind:\n%s", out)
	}
}
