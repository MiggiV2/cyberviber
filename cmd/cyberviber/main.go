// Command cyberviber: Claude Code session manager daemon, hook and CLI.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"cyberviber/internal/config"
	"cyberviber/internal/gitinfo"
	"cyberviber/internal/hook"
	"cyberviber/internal/install"
	"cyberviber/internal/proc"
	"cyberviber/internal/protocol"
	"cyberviber/internal/server"
	"cyberviber/internal/session"
)

const usage = `cyberviber — Claude Code session manager

usage:
  cyberviber daemon       run the daemon (started by launchd after install)
  cyberviber status       list sessions known to the daemon
  cyberviber sim          open the browser simulator (daemon must be running)
  cyberviber install      add hooks to ~/.claude/settings.json and start the launchd agent
  cyberviber uninstall    remove hooks and the launchd agent
  cyberviber hook         (called by Claude Code) forward a hook event to the daemon
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cfg, err := config.Load(config.DefaultPath())
	if err != nil && os.Args[1] != "hook" {
		log.Fatal(err)
	}
	if p, err := strconv.Atoi(os.Getenv("CYBERVIBER_PORT")); err == nil {
		cfg.Port = p
	}

	switch os.Args[1] {
	case "hook":
		runHook(cfg)
	case "daemon":
		err = runDaemon(cfg)
	case "status":
		err = runStatus(cfg)
	case "sim":
		err = exec.Command("open", fmt.Sprintf("http://127.0.0.1:%d/", cfg.Port)).Run()
	case "install":
		err = runInstall(os.Args[2:])
	case "uninstall":
		err = runUninstall(os.Args[2:])
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cyberviber:", err)
		os.Exit(1)
	}
}

// runHook must stay silent on stdout (Claude Code reads it) and always exit 0.
func runHook(cfg config.Config) {
	e, err := hook.Build(os.Stdin, os.Getenv, func() (proc.Proc, bool) { return proc.FindAgent(os.Getppid()) }, time.Now())
	if err != nil {
		return
	}
	_ = hook.Send(cfg.Port, e)
}

func runDaemon(cfg config.Config) error {
	store := session.New(cfg.IdleAfter, gitinfo.Branch)
	statePath := config.StatePath()
	if err := store.Load(statePath); err != nil {
		log.Printf("restore sessions: %v", err)
	}
	save := func() {
		if err := store.Save(statePath); err != nil {
			log.Printf("save sessions: %v", err)
		}
	}
	store.Tick(time.Now(), proc.Alive)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.Port))
	if err != nil {
		return err
	}
	api := server.New(store, save)
	srv := &http.Server{Handler: api.Handler(), ReadHeaderTimeout: 2 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				if store.Tick(now, proc.Alive) {
					save()
					api.Notify()
				}
			}
		}
	}()
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()

	log.Printf("cyberviber daemon listening on %s — simulator: http://%s/", ln.Addr(), ln.Addr())
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	save()
	return nil
}

func runStatus(cfg config.Config) error {
	c := http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/sessions", cfg.Port))
	if err != nil {
		return fmt.Errorf("daemon not reachable on port %d (is it running?)", cfg.Port)
	}
	defer resp.Body.Close()
	var msg protocol.Sessions
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
		return err
	}
	if len(msg.Sessions) == 0 {
		fmt.Println("no claude running")
		return nil
	}
	glyph := map[string]string{"input": "!", "ready": "✓", "working": "✻", "idle": "◦"}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\tNAME\tBRANCH\tSTATE\tTIME\tZELLIJ\tACTIVITY")
	for _, s := range msg.Sessions {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", glyph[s.State], s.Name, s.Branch, s.State,
			elapsed(msg.Now-s.Since), s.Zellij, s.Activity)
	}
	return w.Flush()
}

func elapsed(sec int64) string {
	switch {
	case sec < 60:
		return fmt.Sprintf("%ds", sec)
	case sec < 3600:
		return fmt.Sprintf("%dm%02d", sec/60, sec%60)
	}
	return fmt.Sprintf("%dh%02d", sec/3600, sec%3600/60)
}

type installPaths struct {
	settings, plist, log string
	launchd             bool
}

func parseInstallFlags(name string, args []string) (installPaths, error) {
	home, _ := os.UserHomeDir()
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	p := installPaths{}
	fs.StringVar(&p.settings, "settings", filepath.Join(home, ".claude", "settings.json"), "Claude Code settings file")
	fs.BoolVar(&p.launchd, "launchd", true, "install/remove the launchd agent")
	p.plist = filepath.Join(home, "Library", "LaunchAgents", install.Label+".plist")
	p.log = filepath.Join(home, "Library", "Logs", "cyberviber.log")
	return p, fs.Parse(args)
}

func runInstall(args []string) error {
	p, err := parseInstallFlags("install", args)
	if err != nil {
		return err
	}
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	if bin, err = filepath.EvalSymlinks(bin); err != nil {
		return err
	}
	if strings.Contains(bin, "go-build") {
		return errors.New("refusing to install a `go run` binary; build or `go install` it first")
	}

	if err := rewriteSettings(p.settings, func(b []byte) ([]byte, error) { return install.MergeHooks(b, bin) }); err != nil {
		return err
	}
	fmt.Println("hooks added to", p.settings)

	if !p.launchd {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p.plist), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p.plist, []byte(install.LaunchAgentPlist(bin, p.log)), 0o644); err != nil {
		return err
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", domain+"/"+install.Label).Run()
	if out, err := exec.Command("launchctl", "bootstrap", domain, p.plist).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %v: %s", err, out)
	}
	fmt.Println("launchd agent started:", p.plist)
	fmt.Println("new Claude Code sessions report to the daemon; already running ones after their next hook event")
	return nil
}

func runUninstall(args []string) error {
	p, err := parseInstallFlags("uninstall", args)
	if err != nil {
		return err
	}
	if err := rewriteSettings(p.settings, install.RemoveHooks); err != nil {
		return err
	}
	fmt.Println("hooks removed from", p.settings)
	if p.launchd {
		_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", os.Getuid(), install.Label)).Run()
		if err := os.Remove(p.plist); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		fmt.Println("launchd agent removed")
	}
	return nil
}

// rewriteSettings backs up the settings file once and replaces it atomically.
func rewriteSettings(path string, f func([]byte) ([]byte, error)) error {
	in, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	out, err := f(in)
	if err != nil {
		return err
	}
	if len(in) > 0 {
		backup := path + ".cyberviber-backup"
		if _, err := os.Stat(backup); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(backup, in, 0o600); err != nil {
				return err
			}
			fmt.Println("backup written to", backup)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp := path + ".cyberviber-tmp"
	if err := os.WriteFile(tmp, out, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
