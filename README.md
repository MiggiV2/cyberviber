# cyberviber

Claude Code session manager for a 2.4" ESP32 touch display (ESP32-2432S024, 240×320). It lists all running Claude Code sessions, shows their state (needs you / ready / working / idle), and tapping one brings its terminal to the front: zellij pane, Tabby tab or Ghostty window.

Status: the Mac daemon, the browser simulator and the jump to the terminal work. USB link and ESP32 firmware come next. Design and architecture are in [spec.md](spec.md); the layouts are in [design/index.html](design/index.html).

## Install (macOS)

From source:

```sh
go install github.com/d135-1r43/cyberviber/cmd/cyberviber@latest
~/go/bin/cyberviber install
```

Or download a binary from the [releases](https://github.com/d135-1r43/cyberviber/releases) (arm64, amd64, universal). The binaries are not notarized: run `xattr -d com.apple.quarantine cyberviber` after extracting.

`cyberviber install`:
- adds hooks to `~/.claude/settings.json`, keeping all other hooks. A backup is written once to `settings.json.cyberviber-backup`.
- installs the launchd agent `dev.cyberviber.daemon`, which logs to `~/Library/Logs/cyberviber.log`.
- installs the Tabby plugin, if Tabby is present. Reload or restart Tabby once afterwards.

`cyberviber uninstall` removes all of it.

## Use

```sh
cyberviber status    # session table
cyberviber sim       # browser simulator with live sessions: http://127.0.0.1:7337/
cyberviber version
```

Config: `~/.config/cyberviber/config.toml` (`port`, `idle_after`, `serial`, `baud`).

## Develop

```sh
go test ./...
go build ./cmd/cyberviber
```

## License

MIT
