# cyberviber — spec

A small touch display on the desk that lists every running Claude Code session on the Mac, shows its state at a glance, and brings the right terminal to the front when a session is tapped.

Visual design: [`design/index.html`](design/index.html) (open in a browser).

## 1. Goals

- See all running Claude Code sessions without switching windows.
- Notice immediately when a session needs input (permission prompt or question) or has finished.
- Tap a session → its terminal window/tab is focused on the Mac.
- Look like it belongs on an Omarchy desktop: user picks the **layout** and the **theme**.

Non-goals (for now): typing into sessions from the device, answering permission prompts remotely, multiple Macs, sessions on remote machines.

## 2. Hardware

| | |
|---|---|
| Board | TZT ESP32-2432S024 ("Cheap Yellow Display" family), ESP32-WROOM-32, Wi-Fi + BLE |
| Display | 2.4" TFT, 240×320, ILI9341 (SPI) |
| Touch | Touch controller on the board (variant-dependent: XPT2046 resistive or CST820 capacitive — confirm on the unit) |
| UI library | LVGL 9 |
| Orientation | Portrait, 240×320 |

## 3. Architecture

```
┌──────────── Mac ────────────┐            ┌──── ESP32 ────┐
│ Claude Code ── hooks ──┐    │   Wi-Fi    │               │
│ Claude Code ── hooks ──┼──▶ cyberviberd ◀═══ WebSocket ══▶│ LVGL UI       │
│ Claude Code ── hooks ──┘    │  (JSON)    │ layouts/themes│
│        ▲                    │            └───────────────┘
│        └── focus terminal (AppleScript / CLI)
└─────────────────────────────┘
```

Two components:

1. **`cyberviberd`** — a small daemon on the Mac (launchd agent). It is the source of truth for sessions, serves the device and performs the "jump".
2. **Firmware** — ESP32 + LVGL client. Stateless display: renders what the daemon sends and reports taps.

### 3.1 Session detection (Mac)

Use Claude Code **hooks** (configured in `~/.claude/settings.json`) that post the hook's JSON payload plus terminal info to the daemon (`curl` to `http://127.0.0.1:<port>/event` or a Unix socket). The installer adds them.

| Hook event | Resulting state |
|---|---|
| `SessionStart` | `ready` (new session, waiting for first prompt) |
| `UserPromptSubmit` | `working` |
| `PreToolUse` / `PostToolUse` | `working` (+ update activity text, e.g. `Edit AuthResource.java`) |
| `Notification` (permission / input needed) | `input` |
| `Stop` | `ready` |
| `SessionEnd` | session removed |

The daemon also:
- marks a session `idle` after **10 min** (configurable) without events in state `ready`;
- periodically checks that the session's process is still alive (PID/TTY) and drops dead sessions (crash, closed terminal without `SessionEnd`).

Per session the daemon keeps:

| Field | Source |
|---|---|
| `id` | hook `session_id` |
| `name` | basename of `cwd` |
| `branch` | `git rev-parse --abbrev-ref HEAD` in `cwd` |
| `state` | `input` · `ready` · `working` · `idle` |
| `activity` | short text: last tool + target, or the question/permission text, or `Done · …` |
| `since` | timestamp of the last state change |
| `ctx` | context usage in percent, if derivable from the transcript (optional) |
| `terminal` | `TERM_PROGRAM` (Ghostty, iTerm2, Apple_Terminal, …) |
| `tty` / terminal session id | from the hook process (`tty`, `ITERM_SESSION_ID`, …) — needed for the jump |

### 3.2 Jump to terminal (Mac)

On a tap the device sends the session id; the daemon focuses the matching window/tab:

| Terminal | Method |
|---|---|
| iTerm2 | AppleScript: find session by `ITERM_SESSION_ID`, select tab + window, activate |
| Terminal.app | AppleScript: find tab whose `tty` matches, select, activate |
| Ghostty | Activate app; select tab/window via its scripting/IPC if available — **to be verified** |
| tmux inside any terminal | additionally `tmux select-window/select-pane` for the pane owning the TTY |
| Fallback | activate the terminal app only |

macOS will ask once for Automation/Accessibility permission for the daemon.

### 3.3 Transport (Mac ↔ device)

- Wi-Fi, same LAN. The daemon advertises itself via Bonjour/mDNS as `_cyberviber._tcp`; the device discovers it (manual host/port as fallback).
- One WebSocket connection, JSON messages. The daemon pushes the full session list on every change (lists are tiny, no diffing needed).
- Optional shared token configured on both sides; the daemon binds to the LAN interface only.

Daemon → device:

```json
{ "type": "sessions",
  "now": 1762246140,
  "sessions": [
    { "id": "a1b2", "name": "quarkus-svc", "branch": "fix/cve", "state": "input",
      "activity": "Which Quarkus version: 3.27 LTS or 3.30?",
      "since": 1762246125, "ctx": 47, "terminal": "Ghostty", "tab": 6 }
  ] }
```

Device → daemon:

```json
{ "type": "focus", "id": "a1b2" }
{ "type": "hello", "fw": "0.1.0", "layout": "tiling", "theme": "tokyo-night" }
```

Daemon → device after a focus: `{ "type": "focused", "id": "a1b2", "ok": true }` (device shows the toast; on `ok:false` a short error toast).

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

- Tap a session (row, tile, card, chip, waybar number) → `focus` message, the item flashes in `accent`, a mako-style toast shows `⟶ Ghostty · tab 6` for ~1.5 s.
- Connection states: "searching for cyberviberd…" and "disconnected" screens in the current theme.
- Backlight dims after N minutes without changes; any `input` state wakes it.

### 4.6 Rendering constraints (LVGL)

- Flat rectangles, 1–2 px borders, solid fills only. No blur, gradients or shadows.
- One monospace font (JetBrains Mono or CaskaydiaMono) converted with `lv_font_conv` at 9/10/11/12/13/14/22/64 px; include `✻✢✳✶✽✓◦›⟶↓●` glyphs.
- Blinking and spinner via `lv_timer`.
- Minimum touch target: 22 px (btop); prefer ≥ 26 px.

## 5. Configuration

Mac (`~/.config/cyberviber/config.toml`):

```toml
port = 7337
token = ""            # optional shared secret
idle_after = "10m"
```

Device: Wi-Fi credentials via captive-portal provisioning on first boot (e.g. WiFiManager), daemon host override, layout, theme, brightness.

## 6. Milestones

1. **Design** — HTML mockups of all layouts and themes. ✅
2. **Daemon** — hooks + session store + `GET /sessions` JSON; verify states against real sessions.
3. **Jump** — focus logic for iTerm2, Terminal.app, Ghostty (+ tmux).
4. **Firmware skeleton** — display + touch + LVGL, Wi-Fi provisioning, WebSocket client, one layout (`waybar`).
5. **All layouts + themes** — settings screen, NVS persistence, swipe to switch.
6. **Polish** — mDNS discovery, backlight handling, installer (`cyberviber install` writes hooks + launchd plist).

## 7. Open questions

- Daemon language: Go or Swift (single binary, easy launchd) vs. Node/Python (quick)?
- Ghostty: which reliable way exists to focus a specific tab/window?
- Exact touch controller on this board variant.
- Can context usage (`ctx`) be derived cheaply from the transcript, or drop it?
- Should `ready` sessions auto-decay to `idle`, or stay `ready` until touched?
