// Shared declarations of the cyberviber firmware.
#pragma once

#include <Arduino.h>
#include <lvgl.h>

#define FW_VERSION "0.4.0"

static constexpr int W = 240, H = 320;
static constexpr int MAX_SESSIONS = 16;

// ---------------------------------------------------------------- themes + layouts

struct Theme {
  const char* id;
  const char* name;
  uint32_t bg, bg2, surface, border, fg, muted, accent, red, yellow, green;
};
extern const Theme THEMES[];
extern const int N_THEMES;

struct Layout {
  const char* id;
  const char* name;
  char tag;
  void (*build)(lv_obj_t* root);
};
extern const Layout LAYOUTS[];
extern const int N_LAYOUTS;

extern const Theme* T;  // current theme
extern int curLayout, curTheme;
int findLayout(const char* id);  // -1 if unknown
int findTheme(const char* id);

// ---------------------------------------------------------------- sessions

enum State : uint8_t { S_INPUT, S_READY, S_WORKING, S_IDLE };

struct Session {
  char id[64];
  char name[48];
  char branch[48];
  char activity[160];
  char terminal[16];
  char zellij[32];
  State state;
  int64_t since;
  int tab;
  int ctx;
};

extern Session sessions[MAX_SESSIONS];
extern int nSessions;
extern char focusId[64];  // last tapped session (tiling shows it with the accent border)

int64_t nowMac();
bool macTimeKnown();
void fmtElapsed(char* out, size_t n, int64_t sec);
void fmtWhere(char* out, size_t n, const Session& s);
void fmtWhereShort(char* out, size_t n, const Session& s);
void fmtClock(char* out, size_t n);
void fmtDate(char* out, size_t n);
uint32_t stateColor(State s);
const char* stateGlyph(State s);
void countStates(int count[4]);

// ---------------------------------------------------------------- UI helpers (ui.cpp)

lv_color_t C(uint32_t hex);
lv_obj_t* box(lv_obj_t* parent);                          // transparent, no border, not clickable
lv_obj_t* fill(lv_obj_t* parent, uint32_t color);         // solid box
lv_obj_t* frame(lv_obj_t* parent, uint32_t border, int width);  // transparent box with a border
lv_obj_t* label(lv_obj_t* parent, const lv_font_t* font, uint32_t color, const char* text = "");
lv_obj_t* scroller(lv_obj_t* parent);                     // vertical list; keeps its scroll across rebuilds
void oneLine(lv_obj_t* label, int width);                 // single line, "…" when longer than width
void maxLine(lv_obj_t* label, int width);                 // like oneLine, but no wider than the text

void tapTarget(lv_obj_t* obj, int sessionIndex);  // tap → focus that session
void headerTarget(lv_obj_t* obj);                  // long-press → settings

// labels and objects updated by timers until the next rebuild
lv_obj_t* glyphLabel(lv_obj_t* parent, const lv_font_t* font, int sessionIndex);
lv_obj_t* timeLabel(lv_obj_t* parent, const lv_font_t* font, uint32_t color, int sessionIndex);
lv_obj_t* clockLabel(lv_obj_t* parent, const lv_font_t* font, uint32_t color);
lv_obj_t* dateLabel(lv_obj_t* parent, const lv_font_t* font, uint32_t color);
lv_obj_t* spinLabel(lv_obj_t* parent, const lv_font_t* font, uint32_t color);
void blinkOpa(lv_obj_t* obj);
void blinkBorder(lv_obj_t* obj, uint32_t on, uint32_t off);

lv_obj_t* emptyState(lv_obj_t* parent);

// ---------------------------------------------------------------- app (ui.cpp / main.cpp)

void uiInit();
void rebuild();
void onSessions();  // after a sessions message
void applyConfig(int layout, int theme, bool announce);
void applyBacklight(int brightness, int dimAfter, bool announce);  // -1 keeps a value
void applyUsbBottom(int bottom, bool announce);  // 1 = USB port at the bottom, 0 = top, -1 keeps it
void openSettings();
void cycleLayout(int step);  // +1 next, -1 previous; shows a toast
void showToast(const char* text, uint32_t border);
void showFocusError(const char* id, const char* reason);
void tickSecond();

extern bool gestured;  // the current touch became a swipe: no tap

void sendFocus(const char* id);
void sendHello();
void saveConfig();

extern bool usbBottom;  // portrait turned by 180°, so the USB port points down

// ---------------------------------------------------------------- backlight (backlight.cpp)

extern int brightPct;    // 1–100
extern int dimAfterMin;  // 0 = never dim
void setBacklight(uint8_t level);  // main.cpp: drives the panel PWM
void backlightInit();
bool backlightWake();  // activity; true if the screen was dimmed
void backlightTick();  // once a second
void backlightApply();
