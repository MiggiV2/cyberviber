package registry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShouldReadInteractiveSessions(t *testing.T) {
	// given
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "20169.json"), []byte(`{"pid":20169,"sessionId":"1d5b","cwd":"/Users/m/git/field-proof",
		"kind":"interactive","status":"idle","statusUpdatedAt":1793784805823,"name":"field-proof-75"}`), 0o644)
	os.WriteFile(filepath.Join(dir, "1.json"), []byte(`{"pid":1,"sessionId":"sdk","cwd":"/x","kind":"sdk","status":"busy"}`), 0o644)
	os.WriteFile(filepath.Join(dir, "2.json"), []byte(`garbage`), 0o644)
	os.WriteFile(filepath.Join(dir, "3.json"), []byte(`{"pid":3,"kind":"interactive"}`), 0o644)
	os.WriteFile(filepath.Join(dir, "20169.abc.key"), []byte(`secret`), 0o600)

	// when
	got := Read(dir)

	// then
	if len(got) != 1 {
		t.Fatalf("got %d entries: %+v", len(got), got)
	}
	e := got[0]
	if e.PID != 20169 || e.SessionID != "1d5b" || e.Cwd != "/Users/m/git/field-proof" || e.Status != "idle" || e.Name != "field-proof-75" ||
		!e.StatusAt.Equal(time.UnixMilli(1793784805823)) {
		t.Fatalf("unexpected %+v", e)
	}
}

func TestShouldReturnNothingForMissingDir(t *testing.T) {
	if got := Read(filepath.Join(t.TempDir(), "nope")); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}
