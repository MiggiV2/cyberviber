package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/d135-1r43/cyberviber/internal/install"
)

func launchAgentPaths() (plist, log string) {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", install.Label+".plist"),
		filepath.Join(home, "Library", "Logs", "cyberviber.log")
}

func installService(bin string) error {
	plist, log := launchAgentPaths()
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(plist, []byte(install.LaunchAgentPlist(bin, log)), 0o644); err != nil {
		return err
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", domain+"/"+install.Label).Run()
	// bootout is asynchronous: bootstrap fails with EIO until the old agent is gone
	var out []byte
	var err error
	for i := 0; i < 20; i++ {
		if out, err = exec.Command("launchctl", "bootstrap", domain, plist).CombinedOutput(); err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		return fmt.Errorf("launchctl bootstrap: %v: %s", err, out)
	}
	fmt.Println("launchd agent started:", plist)
	return nil
}

func removeService() error {
	plist, _ := launchAgentPaths()
	_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", os.Getuid(), install.Label)).Run()
	if err := os.Remove(plist); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Println("launchd agent removed")
	return nil
}

func openURL(url string) error { return exec.Command("open", url).Run() }
