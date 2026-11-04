# cyberviber

Claude Code session manager for a 2.4" ESP32 touch display. See [spec.md](spec.md) and the designs in [design/index.html](design/index.html).

## Daemon

```sh
go test ./...
go install ./cmd/cyberviber      # → ~/go/bin/cyberviber
cyberviber install               # hooks in ~/.claude/settings.json + launchd agent
cyberviber status
cyberviber uninstall
```

Config: `~/.config/cyberviber/config.toml` (`port`, `idle_after`, `serial`, `baud`). Log: `~/Library/Logs/cyberviber.log`.
