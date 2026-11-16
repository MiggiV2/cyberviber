package serial

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// HubSocket is where cyd's hub (cyd-launcher) listens; keep it in sync with hub.SocketPath there.
func HubSocket(getenv func(string) string) string {
	return hubSocket(runtime.GOOS, getenv, os.Getuid())
}

func hubSocket(goos string, getenv func(string) string, uid int) string {
	if dir := getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "cyd-hub.sock")
	}
	dir := os.TempDir()
	if goos == "darwin" {
		dir = "/tmp"
	}
	return filepath.Join(dir, fmt.Sprintf("cyd-hub-%d.sock", uid))
}

func isSocket(path string) bool {
	if path == "" {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && fi.Mode()&os.ModeSocket != 0
}

func hubRelease(sock string, d time.Duration) error {
	c, err := net.DialTimeout("unix", sock, time.Second)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	b, _ := json.Marshal(map[string]any{"type": "hub", "cmd": "release", "seconds": int(d / time.Second)})
	if _, err := c.Write(append(b, '\n')); err != nil {
		return err
	}
	var reply struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(c).Decode(&reply); err != nil {
		return err
	}
	if !reply.OK {
		return fmt.Errorf("hub: %s", reply.Error)
	}
	return nil
}
