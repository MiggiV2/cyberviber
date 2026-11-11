package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/d135-1r43/cyberviber/internal/install"
)

const unitName = "cyberviber.service"

func unitPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "systemd", "user", unitName)
}

func systemctl(args ...string) error {
	if out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl --user %v: %v: %s", args, err, out)
	}
	return nil
}

func installService(bin string) error {
	unit := unitPath()
	if err := os.MkdirAll(filepath.Dir(unit), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(unit, []byte(install.SystemdUnit(bin)), 0o644); err != nil {
		return err
	}
	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	if err := systemctl("enable", unitName); err != nil {
		return err
	}
	if err := systemctl("restart", unitName); err != nil {
		return err
	}
	fmt.Println("systemd user service started:", unit, "(logs: journalctl --user -u cyberviber)")
	return nil
}

func removeService() error {
	_ = systemctl("disable", "--now", unitName)
	if err := os.Remove(unitPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_ = systemctl("daemon-reload")
	fmt.Println("systemd user service removed")
	return nil
}

func openURL(url string) error { return exec.Command("xdg-open", url).Run() }
