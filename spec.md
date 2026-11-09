# cyberviber — spec

A small touch display on the desk that lists every running Claude Code session on the Mac, shows its state at a glance, and brings the right terminal to the front when a session is tapped.

Visual design: [`design/index.html`](design/index.html) (open in a browser).

## 1. Goals

- See all running Claude Code sessions without switching windows.
- Notice immediately when a session needs input (permission prompt or question) or has finished.
- Tap a session → its terminal window/tab is focused on the Mac.
- Look like it belongs on an Omarchy desktop: user picks the **layout** and the **theme**.

Non-goals (for now): typing into sessions from the device, answering permission prompts remotely, multiple Macs, sessions on remote machines, wireless operation (Wi-Fi/BLE are a possible later extension; the JSON protocol is transport-independent).

## 2. Hardware

| | |
|---|---|
| Board | TZT ESP32-2432S024 ("Cheap Yellow Display" family), ESP32-WROOM-32, Wi-Fi + BLE |
| Display | 2.4" TFT, 240×320, ILI9341-compatible (SPI: SCLK 14, MOSI 13, MISO 12, DC 2, CS 15, BL 27). **Verified:** the controller memory is 320×240 with BGR order; LovyanGFX `panel_width=320, panel_height=240, rgb_order=true`, rotation 5 = upright portrait with USB at the top |
| Touch | **Verified:** XPT2046 resistive (the "R" variant; no I²C device answers, so not the CST820), on the display's SPI bus with CS 33, IRQ 36. Calibrated once on the device (LovyanGFX four-corner routine), stored in NVS |
| UI library | LVGL 9 |
| Orientation | Portrait, 240×320 |
| Connection | USB (power + serial data to the Mac). Onboard Wi-Fi/BLE unused for now |

## 3. Architecture

```
┌─────────────────── Mac ───────────────────┐        ┌───── ESP32 ─────┐
│                                           │  USB   │                 │
│  Claude Code ─ hooks ─┐                   │ serial │  LVGL UI        │
│  Claude Code ─ hooks ─┼─▶  cyberviberd  ◀═╪════════╪═▶ layouts       │
│  Claude Code ─ hooks ─┘         │         │ JSON + │    themes       │
│                                 ▼         │ power  │                 │
│        focus terminal (AppleScript / CLI) │        │                 │
└───────────────────────────────────────────┘        └─────────────────┘
```

Two components:

1. **`cyberviberd`** — a small daemon on the Mac (launchd agent), written in **Go**. It is the source of truth for sessions, serves the device and performs the "jump".
2. **Firmware** — ESP32 + LVGL client, connected and powered via USB. Stateless display: renders what the daemon sends and reports taps.

### 3.1 Session detection (Mac)

Use Claude Code **hooks** (configured in `~/.claude/settings.json`) that post the hook's JSON payload plus terminal info to the daemon (`curl` to `http://127.0.0.1:<port>/event` or a Unix socket). The installer adds them.

| Hook event | Resulting state |
|---|---|
| `SessionStart` | `ready`, "New session" |
| `UserPromptSubmit` | `working`, "Thinking…"; resets the changed-files count of the turn |
| `PreToolUse` | `working` + activity text (`Edit main.go`, `Bash: Run tests`, `Fetch zellij.dev`, …); Edit/Write/MultiEdit/NotebookEdit add the file to the turn's changed files |
| `PreToolUse` with `AskUserQuestion` | `input`, activity = the question |
| `PermissionRequest` | `input`, "Allow Bash: rm -rf build?" |
| `Notification` | `input` only for `permission_prompt` / `elicitation_dialog` (keeps the more specific `PermissionRequest` text); `idle_prompt` is ignored |
| `PostToolUse` / `PostToolUseFailure` | `working` (e.g. after a permission was granted) |
| `Stop` | `ready`, "Done" or "Done · N files changed" |
| `SessionEnd` | session removed |

`since` only changes when the state changes (a working turn shows the time since the prompt). The `ready → idle` transition keeps `since`, so idle shows how long the session has been untouched. Sessions are persisted to `~/.cache/cyberviber/sessions.json` and restored on daemon restart (dead PIDs are dropped).

The daemon also:
- marks a session `idle` after **10 min** (configurable) without events in state `ready`;
- periodically checks that the `claude` process (PID) is still alive and drops dead sessions (crash, closed terminal without `SessionEnd`).

**Second source: Claude Code's session registry.** Claude Code writes `~/.claude/sessions/<pid>.json` for each running interactive session (`pid`, `sessionId`, `cwd`, `kind`, `status`, `statusUpdatedAt`, …). The format is undocumented, so it is used best-effort; the `*.key` files next to it are never opened. Every 2 s the daemon:
- adds sessions that have not fired a hook yet (e.g. sessions started before `install`, or waiting at the prompt): `busy` → `working`, otherwise `ready`;
- applies a registry status that is newer than the session's last hook event: `busy` moves `ready`/`idle` to `working`, `idle` moves `working` to `ready` (fixes turns interrupted with Esc, where no `Stop` hook fires);
- never sets or clears `input`, only hooks do.

Observed status values: `busy`, `idle`, `shell` (meaning unclear, currently ignored for existing sessions).

**Process identification:** the kernel process name (`p_comm`) of Claude Code is its version (e.g. `2.1.316`), because the binary lives at `~/.local/share/claude/versions/<version>`. `ps` shows `claude` because it prints `argv[0]`. The daemon therefore reads `argv[0]` and the executable path via `kern.procargs2`.

Per session the daemon keeps:

| Field | Source |
|---|---|
| `id` | hook `session_id` |
| `name` | Claude Code's session name from the registry (`name`, e.g. `field-proof-75`, follows `/rename`); basename of `cwd` until the registry has the session |
| `branch` | `git rev-parse --abbrev-ref HEAD` in `cwd` |
| `state` | `input` · `ready` · `working` · `idle` |
| `activity` | short text: last tool + target, or the question/permission text, or `Done · …` |
| `since` | timestamp of the last state change |
| `ctx` | context usage in percent, from Claude Code's status line input (`context_window.used_percentage`); 0 until known |
| `pid` | PID of the `claude` process (found by walking up from the hook process) — liveness + jump |
| `tty` | controlling TTY of the `claude` process |
| `zellij` | `ZELLIJ_SESSION_NAME` + `ZELLIJ_PANE_ID`, if the hook runs inside zellij |
| `terminal` | `ghostty` or `tabby`, resolved from the process tree (see 3.2), **not** from `TERM_PROGRAM` (inside zellij that is inherited from whichever terminal started the zellij server) |
| `tab` | tab index in the terminal window, when the terminal can report it (display only) |

### 3.2 Jump to terminal (Mac)

**Required:** Ghostty and Tabby on macOS, each **with and without zellij**. Other terminals (iTerm2, Terminal.app) and tmux are out of scope for now; for any unknown terminal the fallback is "activate the app".

On a tap the device sends the session id and the daemon runs two steps: **(1)** if the session lives in zellij, focus its pane; **(2)** focus the outer terminal window/tab and bring the app to the front.

#### Step 0: find the outer anchor process

The *anchor* is the process that runs directly in the terminal tab the user sees:

| Setup | Anchor |
|---|---|
| no zellij | the `claude` process |
| zellij | the **zellij client** attached to `ZELLIJ_SESSION_NAME` (the zellij server is detached, so the `claude` process tree does not lead to the terminal) |

Finding the zellij client for a session, in order:
1. `zellij` processes with a controlling TTY (clients; the server has none) whose command line names the session (`zellij attach S`, `zellij a S`, `zellij -s S`, `zellij --session S`).
2. The client's IPC socket: `lsof -a -p <client> -U` shows the peer address of its connection (`->0x5385…`); `lsof -a -p <server> -U` lists the same address with the session socket path `$TMPDIR/zellij-<uid>/contract_version_1/<S>`. **Verified** on zellij 0.45.1 with a client started as plain `zellij a`.
3. If there is exactly one zellij client, use it.
4. No client found → the session is **detached**: open a new Ghostty window running `zellij attach S` (configurable; Tabby: just activate and report it).

From the anchor, walk up the parent chain until the parent is `Ghostty.app` or `Tabby.app`. That decides `terminal`, and the last process below the app (the tab's login shell) is the **tab root**. The anchor's TTY is the **outer TTY**.

#### Step 1: zellij pane (only with zellij)

```
zellij --session S action list-panes --json      # pane → tab
zellij --session S action go-to-tab <tab>         # if focus-pane-id doesn't switch tabs
zellij --session S action focus-pane-id terminal_<ZELLIJ_PANE_ID>
```

**Verified on zellij 0.45.1:** `focus-pane-id` also switches to the pane's tab, so no `go-to-tab` is needed. The pane id comes from the hook (`ZELLIJ_PANE_ID`) or, for sessions found via the registry, from the `claude` process's environment (`kern.procargs2` includes the environment). The daemon runs the **client's own zellij binary** (its executable path), because launchd agents have a minimal `PATH`; fallback: common install locations. Still open: which client is affected when several are attached (the daemon uses the first).

#### Step 2a: Ghostty (AppleScript, Ghostty ≥ 1.3)

Ghostty's AppleScript dictionary has `window → tab → terminal`, with terminal `id`, `name` and `working directory`, and the commands `focus`, `select tab`, `activate window`. There is **no TTY property**, so the daemon has to map outer TTY → Ghostty terminal:

1. **Cached id:** once resolved, the Ghostty terminal `id` is stored with the session and reused while it still exists.
2. **Working directory:** if exactly one Ghostty terminal has the session's `cwd` (no zellij only), use it.
3. **Title marker:** write an OSC 2 title (`\e]2;cv-<random>\a`) to the outer TTY, then poll `every terminal whose name contains "cv-<random>"` for up to ~300 ms. Works with and without zellij because the bytes go straight to the outer terminal. Claude Code / zellij overwrite the title again on their next update.

Then `focus` the terminal and **always** `activate` Ghostty explicitly (focus alone doesn't reliably bring a backgrounded app or another Space to the front). Known pitfall: a second Ghostty process (e.g. started with `ghostty -e`) can take over AppleScript for the bundle id; the daemon detects this and falls back to app activation.

#### Step 2b: Tabby (companion plugin)

Tabby has no AppleScript or CLI to focus a tab, so cyberviber ships a small **Tabby plugin** (`tabby-cyberviber`, in `tabby-plugin/`, embedded in the binary and installed by `cyberviber install` to `~/Library/Application Support/tabby/plugins/node_modules/tabby-cyberviber`):

- Plain JavaScript, no build step: Tabby `require()`s user plugins and compiles Angular modules at runtime (builtin plugins use `__decorate` too). The module gets `AppService` and `HostWindowService` injected via `ctorParameters`.
- One Unix socket per Tabby window: `~/.cyberviber/tabby/<renderer pid>.sock` (0600, dir 0700), removed on unload; the daemon removes sockets nobody listens on.
- One newline-delimited JSON request per connection:
  - `{"cmd":"focus","pids":[…]}` → selects the top-level tab (`AppService.selectTab`) and, for split tabs, the pane (`SplitTabComponent.focus`) whose shell PID (`session.pty.getTruePID()` / `getPID()`, private in the typings but present at runtime) is in `pids`, then `HostWindowService.bringToFront()`. Answer `{"ok":true,"index":3,"title":…}`.
  - `{"cmd":"list"}` → all tabs/panes with title and shell PIDs (debugging).
- The daemon sends the whole chain from anchor to tab root; afterwards it runs `open -a /Applications/Tabby.app` so the app is activated even when another app is frontmost.
- Plugins load at Tabby startup: after installing, reload or restart Tabby once.
- **Verified** on Tabby 1.0.238 + zellij 0.45.1: the module loads, the shell PID matches the tab root, and a tap selects the right tab and pane.

Fallback without the plugin: activate Tabby via `open -a Tabby` (app level only).

#### Matrix

| | no zellij | zellij |
|---|---|---|
| **Ghostty** | anchor = `claude` → AppleScript (cached id / cwd / title marker) | zellij pane → anchor = zellij client → AppleScript (cached id / title marker) |
| **Tabby** | anchor = `claude` → plugin focus by tab root PID | zellij pane → anchor = zellij client → plugin focus by tab root PID |

The daemon answers the device with `focused` (`ok:false` + `reason`, e.g. `detached`, `no-plugin`, `not-found`); the device shows it in the toast. macOS asks once for Automation permission (daemon → Ghostty).

### 3.3 Transport (Mac ↔ device): USB serial

The board is plugged into the Mac anyway for power, so the same cable carries the data. No Wi-Fi, no pairing, no network setup.

- The board's USB-UART bridge (CH340 or CP210x) appears as `/dev/cu.usbserial-*`, `/dev/cu.wchusbserial-*` or `/dev/cu.SLAB_USBtoUART`.
- **Framing:** newline-delimited JSON (one message per line, UTF-8), **115200 baud** 8N1. Lines that don't parse as JSON are ignored on both sides (ESP32 boot ROM and log output share UART0).
- **Discovery:** the daemon scans matching ports, opens each one and waits for a `hello` from the firmware. The first port that answers is the device. It rescans on unplug and reconnects automatically.
- **Auto-reset:** opening the port toggles DTR/RTS, which resets the ESP32 on this board. The daemon opens the port with DTR/RTS held inactive; if the board resets anyway, it just sends `hello` again after boot and receives the full state.
- **Liveness:** the daemon sends the full session list on every change and at least every 5 s. If the device hears nothing for 15 s it shows "waiting for Mac…".
- **Firmware logging:** off by default in release builds, so the line stream stays clean.
- **Flashing:** the daemon holds the serial port open. `cyberviber release [s]` (`POST /release`) frees it for `pio run -t upload`; the daemon reattaches afterwards. **Verified:** release → flash → reattach.
- **Handshake (implemented):** the daemon sends `{"type":"hello","daemon":"cyberviber",…}` every second while probing a port; the firmware answers with its own `hello` (and also says hello once after boot). Probing gives up after 6 s per port. A `log` message from the device (debug builds) goes to the daemon log. `GET /device` reports the connection; `cyberviber status` prints it.
- **Clock:** the `sessions` message carries `tz` (the Mac's UTC offset in seconds) so the device can show local time.
- **Config (implemented):** daemon → device `{"type":"config","layout":"tiling","theme":"nord","brightness":80,"dim_after":10,"usb":"bottom"}` (every field optional; brightness in percent 1–100, dim_after in minutes 0–1440, usb `top` or `bottom` = portrait turned by 180°); the device applies it, stores it in NVS and answers with a `hello` carrying the applied values. `POST /config` validates the ids.
- **Screenshot (implemented, debug tool):** daemon → device `{"type":"screenshot"}` (optional `y`, `rows`); the device reads the panel back and answers with `{"type":"shot","w":240,"h":320,"y":…,"n":4,"data":"<base64 RGB565 LE>"}` strips and `{"type":"shot_end"}`. The daemon holds back session pushes meanwhile (writing during the stream damaged it) and re-requests damaged strips. **Verified:** this panel's read-back is shifted by one byte (24-bit reads arrive as b, r, g); the firmware corrects it. Occasional short bands in screenshots are read-back noise, not on the screen. `GET /screenshot` → PNG, `cyberviber screenshot [file]`.

The session lists are tiny (< 2 KB for 9 sessions), so 115200 baud is plenty; the full list is sent each time, no diffing.

Daemon → device:

```json
{ "type": "sessions",
  "now": 1762246140,
  "sessions": [
    { "id": "a1b2", "name": "quarkus-svc", "branch": "fix/cve", "state": "input",
      "activity": "Which Quarkus version: 3.27 LTS or 3.30?",
      "since": 1762246125, "ctx": 47,
      "terminal": "ghostty", "tab": 6, "zellij": "work" }
  ] }
```

Device → daemon:

```json
{ "type": "focus", "id": "a1b2" }
{ "type": "hello", "device": "cyberviber", "fw": "0.1.0", "layout": "tiling", "theme": "tokyo-night" }
```

Daemon → device after a focus: `{ "type": "focused", "id": "a1b2", "ok": true }` or `{ "type": "focused", "id": "a1b2", "ok": false, "reason": "detached" }` (device shows the toast; on `ok:false` a short error toast).

### 3.4 Daemon implementation (Go)

- Single static binary `cyberviber` with subcommands: `daemon` (run by launchd), `install` / `uninstall` (hooks + launchd plist), `status` (prints the session list), `sim` (opens the browser simulator), `layout <id>`, `theme <id>`, `release` (free the serial port for flashing).
- The hooks call the same binary (`cyberviber hook <event>`). It reads the hook JSON from stdin, adds process info (`claude` PID, TTY, `ZELLIJ_SESSION_NAME`, `ZELLIJ_PANE_ID`) and posts it to the daemon on `127.0.0.1:<port>`. It must exit fast and never block Claude Code, even if the daemon is down.
- Serial: `go.bug.st/serial` (port enumeration with USB VID/PID, DTR/RTS control).
- Config: TOML (`github.com/BurntSushi/toml`). Otherwise the standard library only (`net/http`, `encoding/json`, `os/exec` for `osascript`, `git`, `zellij`, `ps`/`lsof`; process tree via `sysctl kern.proc` where possible).
- Built with `go build`, no CGO; installed via `go install` or a release binary for darwin/arm64 + amd64.
- Suggested layout:

```
cmd/cyberviber/      main + subcommands
internal/session/    session store, state machine, idle/liveness checks
internal/hook/       hook payload parsing, terminal info
internal/serial/     port discovery, line framing, reconnect
internal/proc/       process tree, anchor + tab root resolution, zellij client lookup
internal/focus/      focus strategies: zellij, ghostty (AppleScript), tabby (plugin socket)
internal/install/    settings.json hook entries, launchd plist
tabby-plugin/        Tabby plugin (TypeScript) exposing tab focus over a Unix socket
firmware/            PlatformIO project for the ESP32
design/              HTML mockups
```

### 3.5 Local HTTP API and browser simulator

The daemon listens on `127.0.0.1:<port>` only:

| Endpoint | Purpose |
|---|---|
| `POST /event` | hook events from `cyberviber hook` |
| `POST /ctx` | context usage `{"session_id","pct"}` from `cyberviber statusline` |
| `GET /sessions` | current list as a `sessions` message (same JSON the device gets) |
| `GET /stream` | server-sent events: a `sessions` message on connect and after every change |
| `POST /focus` | `{"id": …}` → `focused` answer (same as the device's tap); `not-implemented` until milestone 3 |
| `GET /` | the **browser simulator**: `design/index.html`, embedded in the binary |

The simulator is the design page in live mode: all five layouts and all themes render the real sessions, and a tap calls `/focus`, exactly like the device. Opened as a plain file it falls back to demo data. All strings from the daemon are HTML-escaped (activity text contains commands and prompts).

Every request must have a loopback `Host` (`127.0.0.1`, `localhost`, `::1`) against DNS rebinding, and if it carries an `Origin` header that origin must match the host, so other websites can't post events or trigger focus. Hook calls send no `Origin`.

## 4. UI

### 4.1 State model (all layouts)

| State | Glyph | Colour (theme slot) | Meaning |
|---|---|---|---|
| `input` | `!` (blinking) | red | Permission prompt or question — needs you |
| `ready` | `✓` | green | Turn finished, your move |
| `working` | spinner `· ✢ ✳ ✶ ✻ ✽` (~120 ms/frame) | yellow | Thinking / running tools |
| `idle` | `◦` | muted | No activity for a while |

**Sort order everywhere:** `input` → `ready` → `working` → `idle`, then most recent state change first.

Elapsed time format: `42s`, `3m07`, `1h05`.

### 4.2 Layouts (user-selectable)

All five designs in `design/index.html` ship. The user picks one; it is persisted.

| id | Name | Character | Capacity |
|---|---|---|---|
| `waybar` | A · waybar | Dense list, 44 px rows, workspace-style indicators in the top bar, summary bar at the bottom | 6 rows, then `+N ↓` (scroll) |
| `tiling` | B · tiling | Hyprland-style tiles with gaps and 2 px borders; `input` tiles blink their border; last tapped tile gets the accent border | 1–9 tiles (1, 1×2, 2×2, 2×3, 3×3; odd last tile spans) |
| `btop` | C · btop | Rounded boxes with inline titles: status meters + session table with context bars | 8 rows, then `↓ N more` |
| `walker` | D · walker | Launcher panel with prompt line, 48 px rows with state icon boxes, first row selected | 5 rows, then `+N ↓` |
| `hyprlock` | E · hyprlock | Top-priority `input`/`ready` session as a big card with "tap to jump" button, others as chips; big clock when nothing needs you | 1 card + 6 chips, then `+N more` |

Every layout handles the empty state ("no claude running").

### 4.3 Themes (user-selectable)

Omarchy themes, each mapped to these slots: `bg`, `bg2`, `surface`, `border`, `fg`, `muted`, `accent`, `red`, `yellow`, `green`. Values are in `design/index.html` (`THEMES`).

Tokyo Night (default), Catppuccin, Gruvbox, Everforest, Kanagawa, Nord, Rosé Pine, Matte Black, Osaka Jade, Ristretto, Catppuccin Latte.

Rules: state colours always come from `red`/`yellow`/`green`/`muted`; `accent` is only used for chrome (focus, selection, toast, titles).

### 4.4 Choosing layout and theme

- **On the device:** long-press (≥ 800 ms) anywhere on the top bar / header → settings screen with two lists (layout, theme) and a live preview; tap to apply. Swipe left/right on the main screen cycles layouts.
- **From the Mac:** `cyberviber layout <id>` / `cyberviber theme <id>` (daemon pushes `{ "type": "config", ... }`).
- Persisted in ESP32 NVS; the device reports it in `hello`.
- Optional: "follow Omarchy" — the daemon reads the current Omarchy theme name (when running on Omarchy/Linux) and pushes it. Not relevant on macOS, kept as an extension point.

### 4.5 Interaction

- The BOOT button (GPIO0) cycles to the next layout, like a left swipe.
- Tap a session (row, tile, card, chip, waybar number) → `focus` message, the item flashes in `accent`, a mako-style toast shows `⟶ Ghostty · tab 6` for ~1.5 s.
- Connection state: a "waiting for Mac…" screen in the current theme until the first message arrives, and again after 15 s of silence.
- Backlight dims after N minutes (`dim_after`, default 10, 0 = never) without activity: a touch, or a session newly in `input`. Either wakes it; the waking touch does nothing else.

### 4.6 Rendering constraints (LVGL)

- Flat rectangles, 1–2 px borders, solid fills only. No blur, gradients or shadows.
- One monospace font (JetBrains Mono or CaskaydiaMono) converted with `lv_font_conv` at 9/10/11/12/13/14/22/64 px; include `✻✢✳✶✽✓◦›⟶↓●` glyphs.
- Blinking and spinner via `lv_timer`.
- Minimum touch target: 22 px (btop); prefer ≥ 26 px.

## 5. Configuration

Mac (`~/.config/cyberviber/config.toml`):

```toml
port = 7337           # localhost only, receives hook events
idle_after = "10m"
serial = "auto"       # or an explicit path like /dev/cu.usbserial-1410
baud = 115200
```

Device (stored in NVS): layout, theme, brightness, dim_after, usb (orientation). Nothing to set up on first boot: plug it in and it works.

## 6. Milestones

1. **Design** — HTML mockups of all layouts and themes. ✅
1b. **Browser simulator** — daemon serves the design page with live sessions via SSE. ✅
2. **Daemon (Go)** — `cyberviber hook` + session store + `cyberviber status` + `install`/`uninstall`. ✅ (ctx via `cyberviber statusline`)
3. **Jump** — Ghostty and Tabby (plugin), each with and without zellij. Tabby + zellij verified end to end; Tabby without zellij and Ghostty (both modes) pending live verification.
4. **Firmware skeleton** — display + touch + LVGL, serial JSON client, one layout (`waybar`). ✅ (`firmware/`, LVGL 9.2 + LovyanGFX + ArduinoJson; USB link in `internal/serial`)
5. **All layouts + themes** — settings screen, NVS persistence, swipe to switch. ✅ (layouts and themes checked with device screenshots; settings screen and swipe checked by hand)
6. **Polish** — backlight handling, serial auto-reconnect, installer (`cyberviber install` writes hooks + launchd plist). ✅ (fw 0.3.0: brightness and dim timeout in the settings screen and via `cyberviber brightness` / `cyberviber dim`; dimming checked by hand)

## 7. Open questions

- zellij: minimum version, tab switching by `focus-pane-id`, behaviour with several attached clients, reliable client → session mapping.
- Owner's zellij setup: installed version, and are sessions started as `zellij attach <name>` / `zellij -s <name>` (easy client → session mapping) or plain `zellij` (random names, needs the socket lookup)?
- Ghostty: reliability of the title-marker approach while Claude Code updates the title; does a newer Ghostty expose a TTY or PID per terminal?
- Ghostty path is implemented and unit-tested, but Ghostty isn't installed on the dev machine: verify the AppleScript property names (`id`, `working directory`, `name`) and the title marker on a real Ghostty ≥ 1.3.
- ~~Can context usage (`ctx`) be derived cheaply from the transcript?~~ Decided: no. The transcript has the token counts but not the context window size (the same model runs with 200k or 1M), so a percentage would be a guess. Claude Code passes the exact `context_window.used_percentage` to its status line command; `install` sets `cyberviber statusline` as the status line (it prints nothing and posts the value to `POST /ctx`). A status line the user already has is kept, and ctx then stays unknown.
- Should `ready` sessions auto-decay to `idle`, or stay `ready` until touched? (currently: decay after `idle_after`)
- Interrupting a turn with Esc: handled via the registry status (see 3.1); verify on a real interrupt.
- Registry status `shell`: what does it mean, and should it map to a state?
- `install` rewrites `settings.json` via encoding/json, which sorts keys. A backup is written once (`settings.json.cyberviber-backup`); key order could be preserved later if it matters.

## 8. References

- Ghostty AppleScript (≥ 1.3): https://ghostty.org/docs/features/applescript (dictionary: `sdef /Applications/Ghostty.app`)
- Ghostty focus pitfalls (explicit `activate`, helper instances taking over AppleScript), one user report with many Claude Code sessions: https://github.com/777genius/claude-notifications-go/issues/116
- ghosttpy, Python wrapper showing the window → tab → terminal model: https://pypi.org/project/ghosttpy/
- zellij CLI actions: https://zellij.dev/documentation/cli-actions
- zellij CLI recipes (`list-panes --json`, `focus-pane-id`, `--session`, `ZELLIJ_PANE_ID`): https://zellij.dev/documentation/cli-recipes.html
- Tabby has no scripted tab focus; other tools fall back to app activation: https://github.com/jiahongc/cc-companion/releases/tag/v1.2.0
- Tabby window focus issue on macOS: https://github.com/Eugeny/tabby/issues/10373
- Claude Code hooks: https://docs.claude.com/en/docs/claude-code/hooks
