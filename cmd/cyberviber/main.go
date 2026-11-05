// Command cyberviber: Claude Code session manager daemon, hook and CLI.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/d135-1r43/cyberviber/internal/config"
	"github.com/d135-1r43/cyberviber/internal/focus"
	"github.com/d135-1r43/cyberviber/internal/gitinfo"
	"github.com/d135-1r43/cyberviber/internal/hook"
	"github.com/d135-1r43/cyberviber/internal/install"
	"github.com/d135-1r43/cyberviber/internal/proc"
	"github.com/d135-1r43/cyberviber/internal/protocol"
	"github.com/d135-1r43/cyberviber/internal/registry"
	"github.com/d135-1r43/cyberviber/internal/serial"
	"github.com/d135-1r43/cyberviber/internal/server"
	"github.com/d135-1r43/cyberviber/internal/session"
)

// version is set at build time (-ldflags "-X main.version=…").
var version = "dev"

const usage = `cyberviber — Claude Code session manager

usage:
  cyberviber daemon       run the daemon (started by launchd after install)
  cyberviber status       list sessions known to the daemon
  cyberviber sim          open the browser simulator (daemon must be running)
  cyberviber layout [id]  show or set the device layout (waybar, tiling, btop, walker, hyprlock)
  cyberviber theme [id]   show or set the device theme (tokyo-night, nord, … see 'theme list')
  cyberviber brightness [1-100]     show or set the backlight brightness in percent
  cyberviber dim [minutes|off]      show or set when the backlight dims without activity
  cyberviber screenshot [file.png]  save the device screen (takes ~20 s)
  cyberviber release [s]  free the device's serial port for s seconds (default 60), e.g. to flash it
  cyberviber install      add hooks to ~/.claude/settings.json and start the launchd agent
  cyberviber uninstall    remove hooks and the launchd agent
  cyberviber version      print the version
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
	case "version", "--version", "-v":
		fmt.Println("cyberviber", version)
	case "hook":
		runHook(cfg)
	case "daemon":
		err = runDaemon(cfg)
	case "status":
		err = runStatus(cfg)
	case "sim":
		err = exec.Command("open", fmt.Sprintf("http://127.0.0.1:%d/", cfg.Port)).Run()
	case "release":
		err = runRelease(cfg, os.Args[2:])
	case "layout":
		err = runConfig(cfg, "layout", protocol.Layouts, os.Args[2:])
	case "theme":
		err = runConfig(cfg, "theme", protocol.Themes, os.Args[2:])
	case "brightness":
		err = runBrightness(cfg, os.Args[2:])
	case "dim":
		err = runDim(cfg, os.Args[2:])
	case "screenshot":
		err = runScreenshot(cfg, os.Args[2:])
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
	store.Sync(discover())

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.Port))
	if err != nil {
		return err
	}
	link := serial.New(cfg.Serial, cfg.Baud)
	link.Version = version
	api := server.New(store, func() { save(); link.Notify() })
	notify := func() { api.Notify(); link.Notify() }
	link.Snapshot = func() []byte { return api.Snapshot() }
	api.Device = func() any { return link.Status() }
	api.Release = link.Release
	api.Configure = func(c protocol.Config) error {
		b, _ := json.Marshal(c)
		return link.Send(b)
	}
	api.Screenshot = func(ctx context.Context) ([]byte, error) {
		img, err := link.Screenshot(ctx)
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		err = png.Encode(&buf, img)
		return buf.Bytes(), err
	}
	focuser := &focus.Focuser{
		Lookup:   store.Get,
		Get:      proc.Get,
		Env:      proc.Env,
		Run:      func(name string, args ...string) ([]byte, error) { return exec.Command(name, args...).CombinedOutput() },
		TabbyDir: focus.TabbyDir(),
	}
	api.Focus = func(id string) (bool, string) {
		res := focuser.Focus(id)
		log.Printf("focus %s: %+v", id, res)
		if res.Terminal != "" && store.SetTerminal(id, res.Terminal, res.Tab) {
			notify()
		}
		return res.OK, res.Reason
	}
	link.Focus = api.Focus
	srv := &http.Server{Handler: api.Handler(), ReadHeaderTimeout: 2 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				synced := store.Sync(discover())
				if store.Tick(now, proc.Alive) || synced {
					save()
					notify()
				}
			}
		}
	}()
	go link.Run(ctx)
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

// discover lists live sessions from Claude Code's session registry. Entries whose PID is gone
// or now belongs to another program are skipped.
func discover() []session.Discovered {
	var out []session.Discovered
	for _, e := range registry.Read(registry.DefaultDir()) {
		p, err := proc.Get(e.PID)
		if err != nil || !p.IsClaude() && p.Name != "node" {
			continue
		}
		env := proc.Env(e.PID)
		out = append(out, session.Discovered{ID: e.SessionID, Cwd: e.Cwd, Name: e.Name, PID: e.PID, TTY: p.TTY,
			Status: e.Status, StatusAt: e.StatusAt,
			ZellijSession: env["ZELLIJ_SESSION_NAME"], ZellijPane: env["ZELLIJ_PANE_ID"]})
	}
	return out
}

func runRelease(cfg config.Config, args []string) error {
	sec := 60
	if len(args) > 0 {
		n, err := strconv.Atoi(args[0])
		if err != nil || n <= 0 {
			return fmt.Errorf("release: seconds must be a positive number, got %q", args[0])
		}
		sec = n
	}
	c := http.Client{Timeout: 2 * time.Second}
	body := strings.NewReader(fmt.Sprintf(`{"seconds":%d}`, sec))
	resp, err := c.Post(fmt.Sprintf("http://127.0.0.1:%d/release", cfg.Port), "application/json", body)
	if err != nil {
		fmt.Println("daemon not running, port is free")
		return nil
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("release: %s", resp.Status)
	}
	// the daemon closes the port asynchronously
	for i := 0; i < 20; i++ {
		var st serial.Status
		if getJSON(cfg, "/device", &st) == nil && !st.Connected {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Printf("serial port released for %ds\n", sec)
	return nil
}

// runConfig shows (no argument), lists ("list") or sets the device's layout or theme.
func runConfig(cfg config.Config, key string, valid []string, args []string) error {
	var st serial.Status
	if err := getJSON(cfg, "/device", &st); err != nil {
		return fmt.Errorf("daemon not reachable on port %d (is it running?)", cfg.Port)
	}
	cur := st.Layout
	if key == "theme" {
		cur = st.Theme
	}
	if len(args) == 0 || args[0] == "list" {
		if len(args) == 0 && !st.Connected {
			return serial.ErrNotConnected
		}
		for _, v := range valid {
			mark := "  "
			if v == cur {
				mark = "* "
			}
			fmt.Println(mark + v)
		}
		return nil
	}
	if !slices.Contains(valid, args[0]) {
		return fmt.Errorf("unknown %s %q (one of: %s)", key, args[0], strings.Join(valid, ", "))
	}
	return postConfig(cfg, map[string]any{key: args[0]}, func(st serial.Status) bool {
		return key == "layout" && st.Layout == args[0] || key == "theme" && st.Theme == args[0]
	}, fmt.Sprintf("%s: %s", key, args[0]))
}

// runBrightness shows or sets the backlight brightness in percent.
func runBrightness(cfg config.Config, args []string) error {
	st, err := deviceStatus(cfg)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		fmt.Printf("brightness: %d%%\n", st.Brightness)
		return nil
	}
	v, err := strconv.Atoi(strings.TrimSuffix(args[0], "%"))
	if err != nil || v < 1 || v > 100 {
		return fmt.Errorf("brightness must be 1–100, got %q", args[0])
	}
	return postConfig(cfg, map[string]any{"brightness": v}, func(st serial.Status) bool { return st.Brightness == v },
		fmt.Sprintf("brightness: %d%%", v))
}

// runDim shows or sets how long the device waits without activity before it dims.
func runDim(cfg config.Config, args []string) error {
	st, err := deviceStatus(cfg)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		fmt.Println("dim after:", fmtDim(st.DimAfter))
		return nil
	}
	v := 0
	if args[0] != "off" && args[0] != "never" && args[0] != "0" {
		d, err := time.ParseDuration(args[0])
		if err != nil { // a plain number means minutes
			n, nerr := strconv.Atoi(args[0])
			d, err = time.Duration(n)*time.Minute, nerr
		}
		v = int(d / time.Minute)
		if err != nil || d%time.Minute != 0 || v < 1 || v > protocol.MaxDimAfter {
			return fmt.Errorf("dim takes whole minutes 1–%d (e.g. 10, 15m, 1h) or off, got %q", protocol.MaxDimAfter, args[0])
		}
	}
	return postConfig(cfg, map[string]any{"dim_after": v}, func(st serial.Status) bool { return st.DimAfter != nil && *st.DimAfter == v },
		"dim after: "+fmtDim(&v))
}

func fmtDim(m *int) string {
	switch {
	case m == nil:
		return "unknown (firmware before 0.3.0)"
	case *m == 0:
		return "off"
	default:
		return fmt.Sprintf("%d min", *m)
	}
}

func deviceStatus(cfg config.Config) (serial.Status, error) {
	var st serial.Status
	if err := getJSON(cfg, "/device", &st); err != nil {
		return st, fmt.Errorf("daemon not reachable on port %d (is it running?)", cfg.Port)
	}
	if !st.Connected {
		return st, serial.ErrNotConnected
	}
	return st, nil
}

// postConfig sends a config change and waits for the device to confirm it with a hello.
func postConfig(cfg config.Config, change map[string]any, confirmed func(serial.Status) bool, done string) error {
	c := http.Client{Timeout: 2 * time.Second}
	body, _ := json.Marshal(change)
	resp, err := c.Post(fmt.Sprintf("http://127.0.0.1:%d/config", cfg.Port), "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	msg, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return errors.New(strings.TrimSpace(string(msg)))
	}
	var st serial.Status
	for i := 0; i < 30; i++ {
		if getJSON(cfg, "/device", &st) == nil && confirmed(st) {
			fmt.Println(done)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("sent, but the device did not confirm it")
}

func runScreenshot(cfg config.Config, args []string) error {
	out := "cyberviber.png"
	if len(args) > 0 {
		out = args[0]
	}
	c := http.Client{Timeout: 70 * time.Second}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/screenshot", cfg.Port))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("screenshot: %s", strings.TrimSpace(string(b)))
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		return err
	}
	fmt.Println("saved", out)
	return nil
}

func getJSON(cfg config.Config, path string, v any) error {
	c := http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d%s", cfg.Port, path))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
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
	var dev serial.Status
	if getJSON(cfg, "/device", &dev) == nil {
		switch {
		case dev.Connected:
			fmt.Printf("device: %s (fw %s)", dev.Port, dev.Firmware)
			if dev.DimAfter != nil {
				fmt.Printf(" · brightness %d%% · dim after %s", dev.Brightness, fmtDim(dev.DimAfter))
			}
			fmt.Print("\n\n")
		case dev.Released > 0:
			fmt.Printf("device: port released until %s\n\n", time.Unix(dev.Released, 0).Format("15:04:05"))
		default:
			fmt.Print("device: not connected\n\n")
		}
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
	launchd              bool
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

	if _, err := os.Stat("/Applications/Tabby.app"); err == nil {
		dir, err := install.InstallTabbyPlugin(install.TabbyPluginsDir())
		if err != nil {
			return err
		}
		fmt.Println("tabby plugin installed:", dir)
		fmt.Println("  → reload Tabby windows to load it (View → Reload, or restart Tabby)")
	}

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
	// bootout is asynchronous: bootstrap fails with EIO until the old agent is gone
	var out []byte
	for i := 0; i < 20; i++ {
		if out, err = exec.Command("launchctl", "bootstrap", domain, p.plist).CombinedOutput(); err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
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
	if err := install.RemoveTabbyPlugin(install.TabbyPluginsDir()); err != nil {
		return err
	}
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
