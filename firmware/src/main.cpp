// cyberviber firmware: renders the daemon's session list (layout A · waybar, Tokyo Night)
// and reports taps. Protocol: newline-delimited JSON over UART0 at 115200 baud (spec §3.3).
#include <Arduino.h>
#include <ArduinoJson.h>
#include <Preferences.h>
#include <lvgl.h>

#include "board.h"

#define FW_VERSION "0.1.0"
#define LAYOUT "waybar"
#define THEME "tokyo-night"

static constexpr int W = 240, H = 320;
static constexpr int MAX_SESSIONS = 16;
static constexpr int VISIBLE_ROWS = 6;
static constexpr int MAX_WS = 9;  // workspace numbers in the top bar
static constexpr uint32_t SILENCE_MS = 15000;
static constexpr uint32_t TOAST_MS = 1600;

static Board tft;

// ---------------------------------------------------------------- theme

struct Theme {
  uint32_t bg, bg2, surface, border, fg, muted, accent, red, yellow, green;
};
static const Theme T = {0x1a1b26, 0x16161e, 0x24283b, 0x3b4261, 0xc0caf5,
                        0x565f89, 0x7aa2f7, 0xf7768e, 0xe0af68, 0x9ece6a};
static lv_color_t C(uint32_t hex) { return lv_color_hex(hex); }

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
};

static Session sessions[MAX_SESSIONS];
static int nSessions = 0;
static int64_t macNow = 0;    // daemon clock at the last message (unix seconds)
static uint32_t macNowAt = 0;  // millis() when it arrived
static int32_t macTZ = 0;
static uint32_t lastHeard = 0;
static bool heard = false;

static const char* SPIN[] = {"·", "✢", "✳", "✶", "✻", "✽", "✻", "✶", "✳", "✢"};
static constexpr int SPIN_N = sizeof(SPIN) / sizeof(SPIN[0]);
static int spinFrame = 0;
static bool blinkOn = true;

static State parseState(const char* s) {
  if (!s) return S_IDLE;
  if (!strcmp(s, "input")) return S_INPUT;
  if (!strcmp(s, "ready")) return S_READY;
  if (!strcmp(s, "working")) return S_WORKING;
  return S_IDLE;
}

static uint32_t stateColor(State s) {
  switch (s) {
    case S_INPUT: return T.red;
    case S_READY: return T.green;
    case S_WORKING: return T.yellow;
    default: return T.muted;
  }
}

static const char* stateGlyph(State s) {
  switch (s) {
    case S_INPUT: return "!";
    case S_READY: return "✓";
    case S_WORKING: return SPIN[spinFrame];
    default: return "◦";
  }
}

// copies at most n-1 bytes without cutting a UTF-8 sequence in half
static void copyUtf8(char* dst, const char* src, size_t n) {
  if (!src) src = "";
  size_t len = strlen(src);
  if (len >= n) {
    len = n - 1;
    while (len > 0 && (src[len] & 0xC0) == 0x80) len--;
  }
  memcpy(dst, src, len);
  dst[len] = 0;
}

static int64_t nowMac() { return macNow + (int64_t)((millis() - macNowAt) / 1000); }

static void fmtElapsed(char* out, size_t n, int64_t sec) {
  if (sec < 0) sec = 0;
  if (sec < 60) snprintf(out, n, "%llds", (long long)sec);
  else if (sec < 3600) snprintf(out, n, "%lldm%02lld", (long long)(sec / 60), (long long)(sec % 60));
  else snprintf(out, n, "%lldh%02lld", (long long)(sec / 3600), (long long)(sec % 3600 / 60));
}

// "Ghostty · zellij work" / "Tabby · tab 3" / "terminal"
static void fmtWhere(char* out, size_t n, const Session& s) {
  char term[16] = "";
  if (s.terminal[0]) {
    copyUtf8(term, s.terminal, sizeof term);
    term[0] = toupper(term[0]);
  }
  char loc[48] = "";
  if (s.zellij[0]) snprintf(loc, sizeof loc, "zellij %s", s.zellij);
  else if (s.tab) snprintf(loc, sizeof loc, "tab %d", s.tab);
  if (term[0] && loc[0]) snprintf(out, n, "%s · %s", term, loc);
  else if (term[0] || loc[0]) snprintf(out, n, "%s", term[0] ? term : loc);
  else snprintf(out, n, "terminal");
}

// ---------------------------------------------------------------- serial out

static void sendHello() {
  Serial.print("{\"type\":\"hello\",\"device\":\"cyberviber\",\"fw\":\"" FW_VERSION
               "\",\"layout\":\"" LAYOUT "\",\"theme\":\"" THEME "\"}\n");
}

static void sendFocus(const char* id) {
  JsonDocument doc;
  doc["type"] = "focus";
  doc["id"] = id;
  serializeJson(doc, Serial);
  Serial.print('\n');
}

// ---------------------------------------------------------------- UI objects

struct Row {
  lv_obj_t *obj, *stripe, *name, *branch, *act, *glyph, *time;
};

static lv_obj_t *bar, *wsBox, *wsLabel[MAX_WS], *clockLabel, *countLabel;
static lv_obj_t* list;
static Row rows[MAX_SESSIONS];
static lv_obj_t *emptyBox, *emptySpin;
static lv_obj_t *foot, *seg, *segPart[4], *sumLabel[4], *moreLabel;
static lv_obj_t *toast, *toastLabel;
static lv_obj_t *waitBox, *waitSpin;
static lv_timer_t* toastTimer;

static constexpr int BAR_H = 20, FOOT_H = 34, ROW_H = 44;
static constexpr int SIDE_W = 44, MAIN_X = 11, MAIN_W = W - MAIN_X - 8 - SIDE_W;

static lv_obj_t* box(lv_obj_t* parent) {
  lv_obj_t* o = lv_obj_create(parent);
  lv_obj_remove_flag(o, LV_OBJ_FLAG_SCROLLABLE);
  lv_obj_remove_flag(o, LV_OBJ_FLAG_CLICKABLE);
  lv_obj_set_style_pad_all(o, 0, 0);
  lv_obj_set_style_border_width(o, 0, 0);
  lv_obj_set_style_radius(o, 0, 0);
  lv_obj_set_style_bg_opa(o, LV_OPA_TRANSP, 0);
  return o;
}

static lv_obj_t* fill(lv_obj_t* parent, uint32_t color) {
  lv_obj_t* o = box(parent);
  lv_obj_set_style_bg_color(o, C(color), 0);
  lv_obj_set_style_bg_opa(o, LV_OPA_COVER, 0);
  return o;
}

static lv_obj_t* label(lv_obj_t* parent, const lv_font_t* font, uint32_t color) {
  lv_obj_t* l = lv_label_create(parent);
  lv_obj_set_style_text_font(l, font, 0);
  lv_obj_set_style_text_color(l, C(color), 0);
  lv_label_set_text(l, "");
  return l;
}

// ---------------------------------------------------------------- toast + flash

static void hideToast(lv_timer_t*) { lv_obj_add_flag(toast, LV_OBJ_FLAG_HIDDEN); }

static void showToast(const char* text, uint32_t border) {
  lv_label_set_text(toastLabel, text);
  lv_obj_set_style_border_color(toast, C(border), 0);
  lv_obj_remove_flag(toast, LV_OBJ_FLAG_HIDDEN);
  lv_obj_move_foreground(toast);
  lv_timer_reset(toastTimer);
  lv_timer_resume(toastTimer);
}

static void flashOpa(void* obj, int32_t v) { lv_obj_set_style_bg_opa((lv_obj_t*)obj, (lv_opa_t)v, 0); }

static void flash(lv_obj_t* obj) {
  lv_obj_set_style_bg_color(obj, C(T.accent), 0);
  lv_anim_t a;
  lv_anim_init(&a);
  lv_anim_set_var(&a, obj);
  lv_anim_set_exec_cb(&a, flashOpa);
  lv_anim_set_values(&a, LV_OPA_COVER, LV_OPA_TRANSP);
  lv_anim_set_duration(&a, 450);
  lv_anim_set_path_cb(&a, lv_anim_path_ease_out);
  lv_anim_start(&a);
}

static void tapSession(int i, lv_obj_t* target) {
  if (i < 0 || i >= nSessions) return;
  sendFocus(sessions[i].id);
  flash(target);
  char where[64], text[96];
  fmtWhere(where, sizeof where, sessions[i]);
  snprintf(text, sizeof text, "⟶ %s", where);
  showToast(text, T.accent);
}

static void onRowClick(lv_event_t* e) {
  tapSession((int)(intptr_t)lv_event_get_user_data(e), (lv_obj_t*)lv_event_get_current_target(e));
}

// ---------------------------------------------------------------- build

static void buildBar(lv_obj_t* scr) {
  bar = fill(scr, T.bg2);
  lv_obj_set_size(bar, W, BAR_H);
  lv_obj_set_style_border_side(bar, LV_BORDER_SIDE_BOTTOM, 0);
  lv_obj_set_style_border_width(bar, 1, 0);
  lv_obj_set_style_border_color(bar, C(T.border), 0);

  wsBox = box(bar);
  lv_obj_set_size(wsBox, LV_SIZE_CONTENT, BAR_H - 1);
  lv_obj_set_pos(wsBox, 2, 0);
  lv_obj_set_flex_flow(wsBox, LV_FLEX_FLOW_ROW);
  for (int i = 0; i < MAX_WS; i++) {
    lv_obj_t* l = label(wsBox, &cv_11b, T.fg);
    lv_obj_set_size(l, 14, BAR_H - 1);
    lv_obj_set_style_text_align(l, LV_TEXT_ALIGN_CENTER, 0);
    lv_obj_set_style_pad_top(l, 3, 0);
    lv_obj_set_style_bg_color(l, C(T.red), 0);
    lv_label_set_text_fmt(l, "%d", i + 1);
    lv_obj_add_flag(l, LV_OBJ_FLAG_CLICKABLE);
    lv_obj_add_event_cb(l, onRowClick, LV_EVENT_CLICKED, (void*)(intptr_t)i);
    wsLabel[i] = l;
  }
  clockLabel = label(bar, &cv_11, T.fg);
  lv_obj_align(clockLabel, LV_ALIGN_CENTER, 0, 0);
  countLabel = label(bar, &cv_11, T.muted);
  lv_obj_align(countLabel, LV_ALIGN_RIGHT_MID, -4, 0);
}

static void buildRow(int i) {
  Row& r = rows[i];
  r.obj = box(list);
  lv_obj_set_size(r.obj, W, ROW_H);
  lv_obj_add_flag(r.obj, LV_OBJ_FLAG_CLICKABLE);
  lv_obj_set_style_border_side(r.obj, LV_BORDER_SIDE_BOTTOM, 0);
  lv_obj_set_style_border_width(r.obj, 1, 0);
  lv_obj_set_style_border_color(r.obj, C(T.surface), 0);
  lv_obj_add_event_cb(r.obj, onRowClick, LV_EVENT_CLICKED, (void*)(intptr_t)i);

  r.stripe = fill(r.obj, T.muted);
  lv_obj_set_size(r.stripe, 3, ROW_H - 1);

  r.name = label(r.obj, &cv_13b, T.fg);
  lv_label_set_long_mode(r.name, LV_LABEL_LONG_DOT);
  lv_obj_set_pos(r.name, MAIN_X, 6);

  r.branch = label(r.obj, &cv_11, T.muted);
  lv_label_set_long_mode(r.branch, LV_LABEL_LONG_DOT);

  r.act = label(r.obj, &cv_11, T.muted);
  lv_label_set_long_mode(r.act, LV_LABEL_LONG_DOT);
  lv_obj_set_width(r.act, MAIN_W);
  lv_obj_set_pos(r.act, MAIN_X, 25);

  r.glyph = label(r.obj, &cv_14b, T.muted);
  lv_obj_align(r.glyph, LV_ALIGN_TOP_RIGHT, -8, 5);
  r.time = label(r.obj, &cv_11, T.muted);
  lv_obj_align(r.time, LV_ALIGN_TOP_RIGHT, -8, 25);
  lv_obj_add_flag(r.obj, LV_OBJ_FLAG_HIDDEN);
}

static lv_obj_t* buildCenter(lv_obj_t* parent, lv_obj_t** spin, const char* head, const char* sub) {
  lv_obj_t* b = box(parent);
  lv_obj_set_size(b, W, LV_SIZE_CONTENT);
  lv_obj_set_flex_flow(b, LV_FLEX_FLOW_COLUMN);
  lv_obj_set_flex_align(b, LV_FLEX_ALIGN_CENTER, LV_FLEX_ALIGN_CENTER, LV_FLEX_ALIGN_CENTER);
  lv_obj_set_style_pad_row(b, 6, 0);
  *spin = label(b, &cv_40, T.accent);
  lv_label_set_text(*spin, "✻");
  lv_obj_t* h = label(b, &cv_13b, T.fg);
  lv_label_set_text(h, head);
  lv_obj_t* s = label(b, &cv_11, T.muted);
  lv_label_set_text(s, sub);
  lv_obj_set_style_text_align(s, LV_TEXT_ALIGN_CENTER, 0);
  return b;
}

static void buildFoot(lv_obj_t* scr) {
  foot = fill(scr, T.bg2);
  lv_obj_set_size(foot, W, FOOT_H);
  lv_obj_set_pos(foot, 0, H - FOOT_H);
  lv_obj_set_style_border_side(foot, LV_BORDER_SIDE_TOP, 0);
  lv_obj_set_style_border_width(foot, 1, 0);
  lv_obj_set_style_border_color(foot, C(T.border), 0);

  seg = fill(foot, T.surface);
  lv_obj_set_size(seg, W - 16, 4);
  lv_obj_set_pos(seg, 8, 6);
  const uint32_t segColor[4] = {T.red, T.green, T.yellow, T.muted};
  for (int i = 0; i < 4; i++) {
    segPart[i] = fill(seg, segColor[i]);
    lv_obj_set_height(segPart[i], 4);
  }

  lv_obj_t* sum = box(foot);
  lv_obj_set_size(sum, W - 16, 14);
  lv_obj_set_pos(sum, 8, 15);
  lv_obj_set_flex_flow(sum, LV_FLEX_FLOW_ROW);
  lv_obj_set_flex_align(sum, LV_FLEX_ALIGN_SPACE_BETWEEN, LV_FLEX_ALIGN_CENTER, LV_FLEX_ALIGN_CENTER);
  for (int i = 0; i < 4; i++) sumLabel[i] = label(sum, &cv_11b, segColor[i]);
  moreLabel = label(sum, &cv_11, T.muted);
}

static void buildUI() {
  lv_obj_t* scr = lv_screen_active();
  lv_obj_remove_flag(scr, LV_OBJ_FLAG_SCROLLABLE);
  lv_obj_set_style_bg_color(scr, C(T.bg), 0);
  lv_obj_set_style_bg_opa(scr, LV_OPA_COVER, 0);
  lv_obj_set_style_text_color(scr, C(T.fg), 0);

  buildBar(scr);

  list = box(scr);
  lv_obj_set_size(list, W, H - BAR_H - FOOT_H);
  lv_obj_set_pos(list, 0, BAR_H);
  lv_obj_add_flag(list, LV_OBJ_FLAG_SCROLLABLE);
  lv_obj_set_scroll_dir(list, LV_DIR_VER);
  lv_obj_set_scrollbar_mode(list, LV_SCROLLBAR_MODE_OFF);
  lv_obj_set_flex_flow(list, LV_FLEX_FLOW_COLUMN);
  for (int i = 0; i < MAX_SESSIONS; i++) buildRow(i);

  emptyBox = buildCenter(scr, &emptySpin, "no claude running", "start one with claude\nin any terminal");
  lv_obj_align(emptyBox, LV_ALIGN_CENTER, 0, -6);

  buildFoot(scr);

  toast = fill(scr, T.bg);
  lv_obj_set_size(toast, W - 28, LV_SIZE_CONTENT);
  lv_obj_align(toast, LV_ALIGN_BOTTOM_MID, 0, -14);
  lv_obj_set_style_border_width(toast, 2, 0);
  lv_obj_set_style_pad_hor(toast, 10, 0);
  lv_obj_set_style_pad_ver(toast, 8, 0);
  toastLabel = label(toast, &cv_11, T.fg);
  lv_obj_set_width(toastLabel, lv_pct(100));
  lv_label_set_long_mode(toastLabel, LV_LABEL_LONG_DOT);
  lv_obj_add_flag(toast, LV_OBJ_FLAG_HIDDEN);
  toastTimer = lv_timer_create(hideToast, TOAST_MS, nullptr);
  lv_timer_pause(toastTimer);
  lv_timer_set_auto_delete(toastTimer, false);
  lv_timer_set_repeat_count(toastTimer, -1);

  waitBox = fill(scr, T.bg);
  lv_obj_set_size(waitBox, W, H);
  lv_obj_add_flag(waitBox, LV_OBJ_FLAG_CLICKABLE);  // swallow taps
  lv_obj_t* w = buildCenter(waitBox, &waitSpin, "waiting for Mac…", "cyberviber daemon\nover USB");
  lv_obj_center(w);
}

// ---------------------------------------------------------------- render

static void renderClock() {
  if (!heard) return;
  int64_t local = nowMac() + macTZ;
  int sod = (int)(((local % 86400) + 86400) % 86400);
  lv_label_set_text_fmt(clockLabel, "%02d:%02d", sod / 3600, sod % 3600 / 60);
}

static void renderTimes() {
  char buf[16];
  int64_t now = nowMac();
  for (int i = 0; i < nSessions; i++) {
    fmtElapsed(buf, sizeof buf, now - sessions[i].since);
    lv_label_set_text(rows[i].time, buf);
  }
}

static void renderGlyphs() {
  for (int i = 0; i < nSessions; i++) {
    const Session& s = sessions[i];
    if (s.state == S_WORKING) lv_label_set_text(rows[i].glyph, SPIN[spinFrame]);
    lv_obj_set_style_opa(rows[i].glyph, s.state == S_INPUT && !blinkOn ? LV_OPA_30 : LV_OPA_COVER, 0);
  }
  if (!lv_obj_has_flag(emptyBox, LV_OBJ_FLAG_HIDDEN)) lv_label_set_text(emptySpin, SPIN[spinFrame]);
  if (!lv_obj_has_flag(waitBox, LV_OBJ_FLAG_HIDDEN)) lv_label_set_text(waitSpin, SPIN[spinFrame]);
}

static void render() {
  int count[4] = {0, 0, 0, 0};
  for (int i = 0; i < nSessions; i++) count[sessions[i].state]++;

  // top bar: one number per session, red block for those that need you
  for (int i = 0; i < MAX_WS; i++) {
    lv_obj_t* l = wsLabel[i];
    if (i >= nSessions) {
      lv_obj_add_flag(l, LV_OBJ_FLAG_HIDDEN);
      continue;
    }
    State st = sessions[i].state;
    lv_obj_remove_flag(l, LV_OBJ_FLAG_HIDDEN);
    lv_obj_set_style_text_color(l, C(st == S_INPUT ? T.bg : stateColor(st)), 0);
    lv_obj_set_style_bg_opa(l, st == S_INPUT ? LV_OPA_COVER : LV_OPA_TRANSP, 0);
  }
  lv_label_set_text_fmt(countLabel, "◉ %d", nSessions);

  for (int i = 0; i < MAX_SESSIONS; i++) {
    Row& r = rows[i];
    if (i >= nSessions) {
      lv_obj_add_flag(r.obj, LV_OBJ_FLAG_HIDDEN);
      continue;
    }
    const Session& s = sessions[i];
    lv_obj_remove_flag(r.obj, LV_OBJ_FLAG_HIDDEN);
    lv_obj_set_style_bg_color(r.stripe, C(stateColor(s.state)), 0);

    lv_obj_set_width(r.name, LV_SIZE_CONTENT);
    lv_label_set_text(r.name, s.name);
    lv_obj_update_layout(r.name);
    int nameW = lv_obj_get_width(r.name);
    if (nameW > MAIN_W) {
      nameW = MAIN_W;
      lv_obj_set_width(r.name, nameW);
    }
    int branchW = MAIN_W - nameW - 6;
    if (s.branch[0] && branchW > 16) {
      lv_label_set_text(r.branch, s.branch);
      lv_obj_set_width(r.branch, LV_SIZE_CONTENT);
      lv_obj_update_layout(r.branch);
      if (lv_obj_get_width(r.branch) > branchW) lv_obj_set_width(r.branch, branchW);
      lv_obj_set_pos(r.branch, MAIN_X + nameW + 6, 8);
      lv_obj_remove_flag(r.branch, LV_OBJ_FLAG_HIDDEN);
    } else {
      lv_obj_add_flag(r.branch, LV_OBJ_FLAG_HIDDEN);
    }

    lv_label_set_text(r.act, s.activity);
    lv_obj_set_style_text_color(r.act, C(s.state == S_INPUT ? T.red : T.muted), 0);
    lv_label_set_text(r.glyph, stateGlyph(s.state));
    lv_obj_set_style_text_color(r.glyph, C(stateColor(s.state)), 0);
  }

  if (nSessions == 0) lv_obj_remove_flag(emptyBox, LV_OBJ_FLAG_HIDDEN);
  else lv_obj_add_flag(emptyBox, LV_OBJ_FLAG_HIDDEN);

  // footer: proportional state bar + counts
  int parts = 0;
  for (int k = 0; k < 4; k++) parts += count[k] ? 1 : 0;
  int avail = (W - 16) - (parts > 0 ? (parts - 1) * 2 : 0);  // 2 px gaps between parts
  int x = 0, done = 0, prevEnd = 0;
  for (int k = 0; k < 4; k++) {
    if (!count[k]) {
      lv_obj_add_flag(segPart[k], LV_OBJ_FLAG_HIDDEN);
      continue;
    }
    done += count[k];
    int end = avail * done / nSessions;
    int w = LV_MAX(1, end - prevEnd);
    prevEnd = end;
    lv_obj_remove_flag(segPart[k], LV_OBJ_FLAG_HIDDEN);
    lv_obj_set_pos(segPart[k], x, 0);
    lv_obj_set_width(segPart[k], w);
    x += w + 2;
  }
  static const char* sumGlyph[4] = {"!", "✓", "✻", "◦"};
  for (int k = 0; k < 4; k++) lv_label_set_text_fmt(sumLabel[k], "%s %d", sumGlyph[k], count[k]);
  if (nSessions > VISIBLE_ROWS) lv_label_set_text_fmt(moreLabel, "+%d ↓", nSessions - VISIBLE_ROWS);
  else lv_label_set_text(moreLabel, " ");

  renderTimes();
  renderClock();
  renderGlyphs();
}

// ---------------------------------------------------------------- timers

static void onSpin(lv_timer_t*) {
  spinFrame = (spinFrame + 1) % SPIN_N;
  for (int i = 0; i < nSessions; i++)
    if (sessions[i].state == S_WORKING) lv_label_set_text(rows[i].glyph, SPIN[spinFrame]);
  if (!lv_obj_has_flag(emptyBox, LV_OBJ_FLAG_HIDDEN)) lv_label_set_text(emptySpin, SPIN[spinFrame]);
  if (!lv_obj_has_flag(waitBox, LV_OBJ_FLAG_HIDDEN)) lv_label_set_text(waitSpin, SPIN[spinFrame]);
}

static void onBlink(lv_timer_t*) {
  blinkOn = !blinkOn;
  for (int i = 0; i < nSessions; i++)
    if (sessions[i].state == S_INPUT)
      lv_obj_set_style_opa(rows[i].glyph, blinkOn ? LV_OPA_COVER : LV_OPA_30, 0);
}

static void onSecond(lv_timer_t*) {
  bool silent = !heard || millis() - lastHeard > SILENCE_MS;
  bool waiting = !lv_obj_has_flag(waitBox, LV_OBJ_FLAG_HIDDEN);
  if (silent && !waiting) {
    lv_obj_remove_flag(waitBox, LV_OBJ_FLAG_HIDDEN);
    lv_obj_move_foreground(waitBox);
  } else if (!silent && waiting) {
    lv_obj_add_flag(waitBox, LV_OBJ_FLAG_HIDDEN);
  }
  renderTimes();
  renderClock();
}

// ---------------------------------------------------------------- protocol in

static void handleSessions(JsonDocument& doc) {
  macNow = doc["now"] | (int64_t)0;
  macNowAt = millis();
  macTZ = doc["tz"] | 0;

  static Session next[MAX_SESSIONS];
  int n = 0;
  for (JsonObject s : doc["sessions"].as<JsonArray>()) {
    if (n == MAX_SESSIONS) break;
    Session& d = next[n++];
    copyUtf8(d.id, s["id"], sizeof d.id);
    copyUtf8(d.name, s["name"], sizeof d.name);
    copyUtf8(d.branch, s["branch"], sizeof d.branch);
    copyUtf8(d.activity, s["activity"], sizeof d.activity);
    copyUtf8(d.terminal, s["terminal"], sizeof d.terminal);
    copyUtf8(d.zellij, s["zellij"], sizeof d.zellij);
    d.state = parseState(s["state"]);
    d.since = s["since"] | (int64_t)0;
    d.tab = s["tab"] | 0;
  }
  // input → ready → working → idle, then most recent state change first
  std::stable_sort(next, next + n, [](const Session& a, const Session& b) {
    if (a.state != b.state) return a.state < b.state;
    return a.since > b.since;
  });
  memcpy(sessions, next, sizeof(Session) * n);
  nSessions = n;
  render();
}

static void handleFocused(JsonDocument& doc) {
  if (doc["ok"] | false) return;
  const char* id = doc["id"] | "";
  const char* reason = doc["reason"] | "failed";
  const char* name = id;
  for (int i = 0; i < nSessions; i++)
    if (!strcmp(sessions[i].id, id)) name = sessions[i].name;
  char text[96];
  snprintf(text, sizeof text, "✗ %s: %s", name, reason);
  showToast(text, T.red);
}

static void handleLine(char* line) {
  if (line[0] != '{') return;  // boot ROM output and other noise
  JsonDocument doc;
  if (deserializeJson(doc, line)) return;
  const char* type = doc["type"] | "";
  if (!strcmp(type, "sessions")) {
    heard = true;
    lastHeard = millis();
    handleSessions(doc);
    onSecond(nullptr);
  } else if (!strcmp(type, "hello")) {
    lastHeard = millis();
    sendHello();
  } else if (!strcmp(type, "focused")) {
    handleFocused(doc);
  }
}

static void pollSerial() {
  static char buf[8 * 1024];
  static size_t len = 0;
  static bool overflow = false;
  while (Serial.available()) {
    char c = (char)Serial.read();
    if (c == '\n') {
      if (!overflow) {
        buf[len] = 0;
        handleLine(buf);
      }
      len = 0;
      overflow = false;
    } else if (len < sizeof buf - 1) {
      buf[len++] = c;
    } else {
      overflow = true;
    }
  }
}

// ---------------------------------------------------------------- LVGL glue

static void flushCb(lv_display_t* disp, const lv_area_t* area, uint8_t* px) {
  uint32_t w = lv_area_get_width(area), h = lv_area_get_height(area);
  tft.startWrite();
  tft.setAddrWindow(area->x1, area->y1, w, h);
  tft.writePixels((lgfx::rgb565_t*)px, w * h);
  tft.endWrite();
  lv_display_flush_ready(disp);
}

static void touchCb(lv_indev_t*, lv_indev_data_t* data) {
  int32_t x, y;
  if (tft.getTouch(&x, &y)) {
    data->state = LV_INDEV_STATE_PRESSED;
    data->point.x = x;
    data->point.y = y;
#ifdef CV_TOUCH_LOG
    static uint32_t last;
    if (millis() - last > 150) {
      last = millis();
      lgfx::touch_point_t raw;
      tft.getTouchRaw(&raw, 1);
      Serial.printf("{\"type\":\"log\",\"raw\":[%d,%d],\"pt\":[%ld,%ld]}\n", raw.x, raw.y, (long)x, (long)y);
    }
#endif
  } else {
    data->state = LV_INDEV_STATE_RELEASED;
  }
}

static uint32_t tickCb() { return millis(); }

// Touch calibration: LovyanGFX's four-corner routine, run once and kept in NVS.
// Holding a finger on the screen while the board starts runs it again.
static void setupTouch() {
  Preferences prefs;
  prefs.begin("cv", false);
  uint16_t cal[8];
  bool have = prefs.getBytes("tcal", cal, sizeof cal) == sizeof cal;
  int32_t x, y;
  bool held = false;
  for (int i = 0; i < 10 && !held; i++, delay(30)) held = tft.getTouch(&x, &y);
  if (!have || held) {
    uint32_t bg = tft.color888(0x1a, 0x1b, 0x26), fg = tft.color888(0xc0, 0xca, 0xf5);
    tft.fillScreen(bg);
    tft.setTextColor(fg, bg);
    tft.setTextDatum(lgfx::middle_center);
    tft.setTextSize(1);
    tft.drawString("touch calibration", W / 2, H / 2 - 10);
    tft.drawString("tap each arrow tip", W / 2, H / 2 + 6);
    while (tft.getTouch(&x, &y)) delay(20);  // wait for release
    tft.calibrateTouch(cal, tft.color888(0x7a, 0xa2, 0xf7), bg, 16);
    prefs.putBytes("tcal", cal, sizeof cal);
  }
  tft.setTouchCalibrate(cal);
  prefs.end();
}

static constexpr size_t DRAW_BUF_BYTES = W * 40 * 2;

void setup() {
  Serial.setRxBufferSize(8192);
  Serial.begin(115200);

  tft.init();
  tft.setRotation(BOARD_ROTATION);
  tft.setBrightness(200);
  tft.fillScreen(TFT_BLACK);
  setupTouch();

  lv_init();
  lv_tick_set_cb(tickCb);
  lv_display_t* disp = lv_display_create(W, H);
  lv_display_set_flush_cb(disp, flushCb);
  void* drawBuf = heap_caps_malloc(DRAW_BUF_BYTES, MALLOC_CAP_DMA | MALLOC_CAP_8BIT);
  lv_display_set_buffers(disp, drawBuf, nullptr, DRAW_BUF_BYTES, LV_DISPLAY_RENDER_MODE_PARTIAL);
  lv_indev_t* indev = lv_indev_create();
  lv_indev_set_type(indev, LV_INDEV_TYPE_POINTER);
  lv_indev_set_read_cb(indev, touchCb);

  buildUI();
  render();
  lv_timer_create(onSpin, 120, nullptr);
  lv_timer_create(onBlink, 600, nullptr);
  lv_timer_create(onSecond, 1000, nullptr);

  sendHello();
}

void loop() {
  pollSerial();
  lv_timer_handler();
  delay(5);
}
