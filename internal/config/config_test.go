package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShouldUseDefaultsWithoutFile(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil || c.Port != 7337 || c.IdleAfter != 10*time.Minute || c.Serial != "auto" || c.Baud != 115200 {
		t.Fatalf("err=%v %+v", err, c)
	}
}

func TestShouldOverrideFromFile(t *testing.T) {
	// given
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte("port = 9000\nidle_after = \"30m\"\n"), 0o644)

	// when
	c, err := Load(p)

	// then
	if err != nil || c.Port != 9000 || c.IdleAfter != 30*time.Minute || c.Baud != 115200 {
		t.Fatalf("err=%v %+v", err, c)
	}
}

func TestShouldRejectBadDuration(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte("idle_after = \"soon\"\n"), 0o644)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error")
	}
}
