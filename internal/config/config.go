// Package config loads ~/.config/cyberviber/config.toml.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Port      int
	IdleAfter time.Duration
	Serial    string
	Baud      int
}

type file struct {
	Port      int    `toml:"port"`
	IdleAfter string `toml:"idle_after"`
	Serial    string `toml:"serial"`
	Baud      int    `toml:"baud"`
}

func Default() Config {
	return Config{Port: 7337, IdleAfter: 10 * time.Minute, Serial: "auto", Baud: 115200}
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "cyberviber", "config.toml")
}

// StatePath is where the daemon persists sessions across restarts.
func StatePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "cyberviber", "sessions.json")
}

// Load reads path on top of the defaults. A missing file yields the defaults.
func Load(path string) (Config, error) {
	c := Default()
	var f file
	_, err := toml.DecodeFile(path, &f)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if f.Port != 0 {
		c.Port = f.Port
	}
	if f.IdleAfter != "" {
		d, err := time.ParseDuration(f.IdleAfter)
		if err != nil {
			return c, fmt.Errorf("%s: idle_after: %w", path, err)
		}
		c.IdleAfter = d
	}
	if f.Serial != "" {
		c.Serial = f.Serial
	}
	if f.Baud != 0 {
		c.Baud = f.Baud
	}
	return c, nil
}
