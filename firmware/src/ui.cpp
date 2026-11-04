// UI plumbing shared by all layouts: widgets, live labels, taps, toast, waiting screen,
// rebuilds and the settings screen.
#include <time.h>

#include "app.h"

// ---------------------------------------------------------------- formatting

static const char* SPIN[] = {"·", "✢", "✳", "✶", "✻", "✽", "✻", "✶", "✳", "✢"};
static constexpr int SPIN_N = sizeof(SPIN) / sizeof(SPIN[0]);
static int spinFrame = 0;
static bool blinkOn = true;

lv_color_t C(uint32_t hex) { return lv_color_hex(hex); }

uint32_t stateColor(State s) {
  switch (s) {
    case S_INPUT: return T->red;
    case S_READY: return T->green;
    case S_WORKING: return T->yellow;
    default: return T->muted;
  }
}

const char* stateGlyph(State s) {
  switch (s) {
    case S_INPUT: return "!";
    case S_READY: return "✓";
    case S_WORKING: return SPIN[spinFrame];
    default: return "◦";
  }
}

void countStates(int count[4]) {
  for (int k = 0; k < 4; k++) count[k] = 0;
  for (int i = 0; i < nSessions; i++) count[sessions[i].state]++;
}

void fmtElapsed(char* out, size_t n, int64_t sec) {
  if (sec < 0) sec = 0;
  if (sec < 60) snprintf(out, n, "%llds", (long long)sec);
  else if (sec < 3600) snprintf(out, n, "%lldm%02lld", (long long)(sec / 60), (long long)(sec % 60));
  else snprintf(out, n, "%lldh%02lld", (long long)(sec / 3600), (long long)(sec % 3600 / 60));
}

static void termName(char* out, size_t n, const Session& s) {
  snprintf(out, n, "%s", s.terminal);
  if (out[0]) out[0] = toupper(out[0]);
}

// "Ghostty · zellij work" / "Tabby · tab 3" / "terminal"
void fmtWhere(char* out, size_t n, const Session& s) {
  char term[16], loc[48] = "";
  termName(term, sizeof term, s);
  if (s.zellij[0]) snprintf(loc, sizeof loc, "zellij %s", s.zellij);
  else if (s.tab) snprintf(loc, sizeof loc, "tab %d", s.tab);
  if (term[0] && loc[0]) snprintf(out, n, "%s · %s", term, loc);
  else if (term[0] || loc[0]) snprintf(out, n, "%s", term[0] ? term : loc);
  else snprintf(out, n, "terminal");
}

// "zj:work" / "tab 3" / "Ghostty"
void fmtWhereShort(char* out, size_t n, const Session& s) {
  if (s.zellij[0]) snprintf(out, n, "zj:%s", s.zellij);
  else if (s.tab) snprintf(out, n, "tab %d", s.tab);
  else termName(out, n, s);
}

extern int32_t macTZ;

static bool localTime(struct tm* tm) {
  if (!macTimeKnown()) return false;
  time_t t = (time_t)(nowMac() + macTZ);
  gmtime_r(&t, tm);
  return true;
}

void fmtClock(char* out, size_t n) {
  struct tm tm;
  if (localTime(&tm)) strftime(out, n, "%H:%M", &tm);
  else snprintf(out, n, "--:--");
}

void fmtDate(char* out, size_t n) {
  struct tm tm;
  if (!localTime(&tm)) {
    out[0] = 0;
    return;
  }
  strftime(out, n, "%a %d %b", &tm);
  for (char* p = out; *p; p++) *p = tolower(*p);
}

// ---------------------------------------------------------------- widgets

lv_obj_t* box(lv_obj_t* parent) {
  lv_obj_t* o = lv_obj_create(parent);
  lv_obj_remove_flag(o, LV_OBJ_FLAG_SCROLLABLE);
  lv_obj_remove_flag(o, LV_OBJ_FLAG_CLICKABLE);
  lv_obj_set_style_pad_all(o, 0, 0);
  lv_obj_set_style_border_width(o, 0, 0);
  lv_obj_set_style_radius(o, 0, 0);
  lv_obj_set_style_bg_opa(o, LV_OPA_TRANSP, 0);
  return o;
}

lv_obj_t* fill(lv_obj_t* parent, uint32_t color) {
  lv_obj_t* o = box(parent);
  lv_obj_set_style_bg_color(o, C(color), 0);
  lv_obj_set_style_bg_opa(o, LV_OPA_COVER, 0);
  return o;
}

lv_obj_t* frame(lv_obj_t* parent, uint32_t border, int width) {
  lv_obj_t* o = box(parent);
  lv_obj_set_style_border_color(o, C(border), 0);
  lv_obj_set_style_border_width(o, width, 0);
  return o;
}

lv_obj_t* label(lv_obj_t* parent, const lv_font_t* font, uint32_t color, const char* text) {
  lv_obj_t* l = lv_label_create(parent);
  lv_obj_set_style_text_font(l, font, 0);
  lv_obj_set_style_text_color(l, C(color), 0);
  lv_label_set_text(l, text);
  return l;
}

// LVGL only ends a label with "…" when its height is fixed; otherwise it wraps.
void oneLine(lv_obj_t* l, int width) {
  lv_label_set_long_mode(l, LV_LABEL_LONG_DOT);
  lv_obj_set_size(l, width, lv_font_get_line_height(lv_obj_get_style_text_font(l, 0)));
}

void maxLine(lv_obj_t* l, int width) {
  lv_obj_update_layout(l);
  if (lv_obj_get_width(l) > width) oneLine(l, width);
}

// ---------------------------------------------------------------- live labels

enum DynKind : uint8_t { D_TIME, D_SPIN, D_BLINK_OPA, D_BLINK_BORDER, D_CLOCK, D_DATE };
struct Dyn {
  lv_obj_t* obj;
  DynKind kind;
  int8_t idx;
  uint32_t on, off;
};
static constexpr int MAX_DYN = 160;
static Dyn dyn[MAX_DYN];
static int nDyn = 0;

static void addDyn(lv_obj_t* obj, DynKind kind, int idx = -1, uint32_t on = 0, uint32_t off = 0) {
  if (nDyn < MAX_DYN) dyn[nDyn++] = {obj, kind, (int8_t)idx, on, off};
}

lv_obj_t* glyphLabel(lv_obj_t* parent, const lv_font_t* font, int i) {
  State st = sessions[i].state;
  lv_obj_t* l = label(parent, font, stateColor(st), stateGlyph(st));
  if (st == S_WORKING) addDyn(l, D_SPIN);
  if (st == S_INPUT) addDyn(l, D_BLINK_OPA);
  return l;
}

lv_obj_t* timeLabel(lv_obj_t* parent, const lv_font_t* font, uint32_t color, int i) {
  lv_obj_t* l = label(parent, font, color);
  addDyn(l, D_TIME, i);
  return l;
}

lv_obj_t* clockLabel(lv_obj_t* parent, const lv_font_t* font, uint32_t color) {
  lv_obj_t* l = label(parent, font, color, "--:--");
  addDyn(l, D_CLOCK);
  return l;
}

lv_obj_t* dateLabel(lv_obj_t* parent, const lv_font_t* font, uint32_t color) {
  lv_obj_t* l = label(parent, font, color);
  addDyn(l, D_DATE);
  return l;
}

lv_obj_t* spinLabel(lv_obj_t* parent, const lv_font_t* font, uint32_t color) {
  lv_obj_t* l = label(parent, font, color, SPIN[spinFrame]);
  addDyn(l, D_SPIN);
  return l;
}

void blinkOpa(lv_obj_t* obj) { addDyn(obj, D_BLINK_OPA); }

void blinkBorder(lv_obj_t* obj, uint32_t on, uint32_t off) { addDyn(obj, D_BLINK_BORDER, -1, on, off); }

static void updateDyn(bool times, bool spin, bool blink) {
  char buf[24];
  int64_t now = nowMac();
  for (int i = 0; i < nDyn; i++) {
    Dyn& d = dyn[i];
    switch (d.kind) {
      case D_TIME:
        if (times && d.idx < nSessions) {
          fmtElapsed(buf, sizeof buf, now - sessions[d.idx].since);
          lv_label_set_text(d.obj, buf);
        }
        break;
      case D_CLOCK:
        if (times) {
          fmtClock(buf, sizeof buf);
          lv_label_set_text(d.obj, buf);
        }
        break;
      case D_DATE:
        if (times) {
          fmtDate(buf, sizeof buf);
          lv_label_set_text(d.obj, buf);
        }
        break;
      case D_SPIN:
        if (spin) lv_label_set_text(d.obj, SPIN[spinFrame]);
        break;
      case D_BLINK_OPA:
        if (blink) lv_obj_set_style_opa(d.obj, blinkOn ? LV_OPA_COVER : LV_OPA_30, 0);
        break;
      case D_BLINK_BORDER:
        if (blink) lv_obj_set_style_border_color(d.obj, C(blinkOn ? d.on : d.off), 0);
        break;
    }
  }
}

// ---------------------------------------------------------------- scroll keeping

static lv_obj_t* scrollObj = nullptr;
static int32_t savedScroll = 0;
static int savedScrollLayout = -1;

lv_obj_t* scroller(lv_obj_t* parent) {
  lv_obj_t* o = box(parent);
  lv_obj_add_flag(o, LV_OBJ_FLAG_SCROLLABLE);
  lv_obj_set_scroll_dir(o, LV_DIR_VER);
  lv_obj_set_scrollbar_mode(o, LV_SCROLLBAR_MODE_OFF);
  lv_obj_set_flex_flow(o, LV_FLEX_FLOW_COLUMN);
  scrollObj = o;
  return o;
}

// ---------------------------------------------------------------- toast + flash + taps

static lv_obj_t *toast, *toastLabel;
static lv_timer_t* toastTimer;
bool gestured = false;

static void hideToast(lv_timer_t* t) {
  lv_obj_add_flag(toast, LV_OBJ_FLAG_HIDDEN);
  lv_timer_pause(t);
}

void showToast(const char* text, uint32_t border) {
  lv_obj_set_style_bg_color(toast, C(T->bg), 0);
  lv_obj_set_style_border_color(toast, C(border), 0);
  lv_obj_set_style_text_color(toastLabel, C(T->fg), 0);
  lv_label_set_text(toastLabel, text);
  lv_obj_remove_flag(toast, LV_OBJ_FLAG_HIDDEN);
  lv_obj_move_foreground(toast);
  lv_timer_reset(toastTimer);
  lv_timer_resume(toastTimer);
}

void showFocusError(const char* id, const char* reason) {
  const char* name = id;
  for (int i = 0; i < nSessions; i++)
    if (!strcmp(sessions[i].id, id)) name = sessions[i].name;
  char text[96];
  snprintf(text, sizeof text, "✗ %s: %s", name, reason);
  showToast(text, T->red);
}

static void flashOpa(void* obj, int32_t v) { lv_obj_set_style_bg_opa((lv_obj_t*)obj, (lv_opa_t)v, 0); }
static void flashDone(lv_anim_t* a) { lv_obj_delete_async((lv_obj_t*)a->var); }

// an accent overlay over the tapped object that fades out (CSS: .flash)
static void flash(lv_obj_t* target) {
  lv_obj_update_layout(target);
  lv_obj_t* ov = fill(target, T->accent);
  lv_obj_add_flag(ov, LV_OBJ_FLAG_FLOATING);
  int32_t bw = lv_obj_get_style_border_width(target, 0);
  lv_obj_set_pos(ov, -lv_obj_get_style_pad_left(target, 0) - bw, -lv_obj_get_style_pad_top(target, 0) - bw);
  lv_obj_set_size(ov, lv_obj_get_width(target), lv_obj_get_height(target));
  lv_anim_t a;
  lv_anim_init(&a);
  lv_anim_set_var(&a, ov);
  lv_anim_set_exec_cb(&a, flashOpa);
  lv_anim_set_values(&a, LV_OPA_70, LV_OPA_TRANSP);
  lv_anim_set_duration(&a, 450);
  lv_anim_set_path_cb(&a, lv_anim_path_ease_out);
  lv_anim_set_completed_cb(&a, flashDone);
  lv_anim_start(&a);
}

static void rebuildLater(lv_timer_t*) { rebuild(); }

static void onTap(lv_event_t* e) {
  if (gestured) return;
  int i = (int)(intptr_t)lv_event_get_user_data(e);
  if (i < 0 || i >= nSessions) return;
  const Session& s = sessions[i];
  sendFocus(s.id);
  flash((lv_obj_t*)lv_event_get_current_target(e));
  char where[64], text[96];
  fmtWhere(where, sizeof where, s);
  snprintf(text, sizeof text, "⟶ %s", where);
  showToast(text, T->accent);
  if (strcmp(focusId, s.id)) {
    snprintf(focusId, sizeof focusId, "%s", s.id);
    if (!strcmp(LAYOUTS[curLayout].id, "tiling")) {  // move the focus border after the flash
      lv_timer_t* t = lv_timer_create(rebuildLater, 460, nullptr);
      lv_timer_set_repeat_count(t, 1);
    }
  }
}

void tapTarget(lv_obj_t* obj, int i) {
  lv_obj_add_flag(obj, LV_OBJ_FLAG_CLICKABLE);
  lv_obj_add_event_cb(obj, onTap, LV_EVENT_SHORT_CLICKED, (void*)(intptr_t)i);
}

static void onHeaderLong(lv_event_t*) {
  if (!gestured) openSettings();
}

void headerTarget(lv_obj_t* obj) {
  lv_obj_add_flag(obj, LV_OBJ_FLAG_CLICKABLE);
  lv_obj_add_event_cb(obj, onHeaderLong, LV_EVENT_LONG_PRESSED, nullptr);
}

// ---------------------------------------------------------------- empty + waiting

static lv_obj_t* centerBlock(lv_obj_t* parent, const char* head, const char* sub) {
  lv_obj_t* b = box(parent);
  lv_obj_set_size(b, W, LV_SIZE_CONTENT);
  lv_obj_set_flex_flow(b, LV_FLEX_FLOW_COLUMN);
  lv_obj_set_flex_align(b, LV_FLEX_ALIGN_CENTER, LV_FLEX_ALIGN_CENTER, LV_FLEX_ALIGN_CENTER);
  lv_obj_set_style_pad_row(b, 6, 0);
  spinLabel(b, &cv_40, T->accent);
  label(b, &cv_13b, T->fg, head);
  lv_obj_t* s = label(b, &cv_11, T->muted, sub);
  lv_obj_set_style_text_align(s, LV_TEXT_ALIGN_CENTER, 0);
  return b;
}

lv_obj_t* emptyState(lv_obj_t* parent) {
  lv_obj_t* b = centerBlock(parent, "no claude running", "start one with claude\nin any terminal");
  lv_obj_align(b, LV_ALIGN_CENTER, 0, -6);
  return b;
}

static lv_obj_t* waitBox = nullptr;
static lv_obj_t* waitSpin = nullptr;
static constexpr uint32_t SILENCE_MS = 15000;
extern uint32_t lastHeard;
extern bool heard;

static void buildWait() {
  if (waitBox) lv_obj_delete(waitBox);
  waitBox = fill(lv_layer_top(), T->bg);
  lv_obj_set_size(waitBox, W, H);
  lv_obj_add_flag(waitBox, LV_OBJ_FLAG_CLICKABLE);  // swallow taps
  int before = nDyn;
  lv_obj_t* b = centerBlock(waitBox, "waiting for Mac…", "cyberviber daemon\nover USB");
  lv_obj_center(b);
  waitSpin = dyn[before].obj;  // the spinner is not part of the rebuilt screen
  nDyn = before;
  lv_obj_add_flag(waitBox, LV_OBJ_FLAG_HIDDEN);
}

// ---------------------------------------------------------------- rebuild

static lv_obj_t* root = nullptr;
static lv_obj_t* settingsObj = nullptr;

void rebuild() {
  if (scrollObj && savedScrollLayout == curLayout) savedScroll = lv_obj_get_scroll_y(scrollObj);
  else savedScroll = 0;
  scrollObj = nullptr;
  nDyn = 0;
  lv_obj_clean(root);
  lv_obj_set_style_bg_color(root, C(T->bg), 0);
  LAYOUTS[curLayout].build(root);
  if (scrollObj && savedScroll > 0) {
    lv_obj_update_layout(scrollObj);
    lv_obj_scroll_to_y(scrollObj, savedScroll, LV_ANIM_OFF);
  }
  savedScrollLayout = curLayout;
  updateDyn(true, true, true);
}

static uint32_t lastSig = 0;
static bool built = false;

// FNV-1a over everything a layout shows except the running times
static uint32_t signature() {
  uint32_t h = 2166136261u;
  auto mix = [&](const void* p, size_t n) {
    const uint8_t* b = (const uint8_t*)p;
    for (size_t i = 0; i < n; i++) h = (h ^ b[i]) * 16777619u;
  };
  mix(&nSessions, sizeof nSessions);
  for (int i = 0; i < nSessions; i++) {
    const Session& s = sessions[i];
    mix(s.id, strlen(s.id));
    mix(s.name, strlen(s.name));
    mix(s.branch, strlen(s.branch));
    mix(s.activity, strlen(s.activity));
    mix(s.terminal, strlen(s.terminal));
    mix(s.zellij, strlen(s.zellij));
    mix(&s.state, sizeof s.state);
    mix(&s.since, sizeof s.since);
    mix(&s.tab, sizeof s.tab);
    mix(&s.ctx, sizeof s.ctx);
  }
  return h;
}

void onSessions() {
  uint32_t sig = signature();
  if (!built || sig != lastSig) {
    lastSig = sig;
    built = true;
    rebuild();
  }
  tickSecond();
}

void tickSecond() {
  bool silent = !heard || millis() - lastHeard > SILENCE_MS;
  bool waiting = !lv_obj_has_flag(waitBox, LV_OBJ_FLAG_HIDDEN);
  if (silent && !waiting) lv_obj_remove_flag(waitBox, LV_OBJ_FLAG_HIDDEN);
  else if (!silent && waiting) lv_obj_add_flag(waitBox, LV_OBJ_FLAG_HIDDEN);
  updateDyn(true, false, false);
}

static void onSpin(lv_timer_t*) {
  spinFrame = (spinFrame + 1) % SPIN_N;
  updateDyn(false, true, false);
  if (!lv_obj_has_flag(waitBox, LV_OBJ_FLAG_HIDDEN)) lv_label_set_text(waitSpin, SPIN[spinFrame]);
}

static void onBlink(lv_timer_t*) {
  blinkOn = !blinkOn;
  updateDyn(false, false, true);
}

static void onSecond(lv_timer_t*) { tickSecond(); }

// ---------------------------------------------------------------- settings

static void buildSettings();

static void closeSettings(lv_event_t*) {
  if (settingsObj) lv_obj_delete(settingsObj);
  settingsObj = nullptr;
}

static void onPickLayout(lv_event_t* e) {
  applyConfig((int)(intptr_t)lv_event_get_user_data(e), curTheme, true);
}

static void onPickTheme(lv_event_t* e) {
  applyConfig(curLayout, (int)(intptr_t)lv_event_get_user_data(e), true);
}

static lv_obj_t* settingsRow(lv_obj_t* list, bool on, lv_event_cb_t cb, int value) {
  lv_obj_t* r = frame(list, on ? T->accent : T->border, 1);
  lv_obj_set_size(r, lv_pct(100), 28);
  lv_obj_add_flag(r, LV_OBJ_FLAG_CLICKABLE);
  lv_obj_add_event_cb(r, cb, LV_EVENT_SHORT_CLICKED, (void*)(intptr_t)value);
  if (on) {
    lv_obj_t* tick = label(r, &cv_11b, T->accent, "✓");
    lv_obj_align(tick, LV_ALIGN_RIGHT_MID, -8, 0);
  }
  return r;
}

static void buildSettings() {
  int32_t scroll = 0;
  if (settingsObj) {
    scroll = lv_obj_get_scroll_y(lv_obj_get_child(settingsObj, 1));
    lv_obj_delete(settingsObj);
  }
  settingsObj = fill(lv_screen_active(), T->bg);
  lv_obj_set_size(settingsObj, W, H);
  lv_obj_add_flag(settingsObj, LV_OBJ_FLAG_CLICKABLE);

  lv_obj_t* head = fill(settingsObj, T->bg2);
  lv_obj_set_size(head, W, 30);
  lv_obj_set_style_border_side(head, LV_BORDER_SIDE_BOTTOM, 0);
  lv_obj_set_style_border_width(head, 1, 0);
  lv_obj_set_style_border_color(head, C(T->border), 0);
  lv_obj_t* chev = label(head, &cv_13b, T->accent, "›");
  lv_obj_align(chev, LV_ALIGN_LEFT_MID, 10, 0);
  lv_obj_t* title = label(head, &cv_13b, T->fg, "settings");
  lv_obj_align(title, LV_ALIGN_LEFT_MID, 22, 0);
  lv_obj_t* done = frame(head, T->accent, 1);
  lv_obj_set_size(done, 56, 22);
  lv_obj_align(done, LV_ALIGN_RIGHT_MID, -6, 0);
  lv_obj_add_flag(done, LV_OBJ_FLAG_CLICKABLE);
  lv_obj_add_event_cb(done, closeSettings, LV_EVENT_SHORT_CLICKED, nullptr);
  lv_obj_t* dl = label(done, &cv_11b, T->accent, "done");
  lv_obj_center(dl);

  lv_obj_t* list = box(settingsObj);
  lv_obj_set_pos(list, 0, 30);
  lv_obj_set_size(list, W, H - 30);
  lv_obj_add_flag(list, LV_OBJ_FLAG_SCROLLABLE);
  lv_obj_set_scroll_dir(list, LV_DIR_VER);
  lv_obj_set_scrollbar_mode(list, LV_SCROLLBAR_MODE_OFF);
  lv_obj_set_flex_flow(list, LV_FLEX_FLOW_COLUMN);
  lv_obj_set_style_pad_all(list, 8, 0);
  lv_obj_set_style_pad_row(list, 4, 0);

  // live preview of the state colours in the current theme
  lv_obj_t* prev = frame(list, T->border, 1);
  lv_obj_set_size(prev, lv_pct(100), 26);
  lv_obj_set_flex_flow(prev, LV_FLEX_FLOW_ROW);
  lv_obj_set_flex_align(prev, LV_FLEX_ALIGN_SPACE_EVENLY, LV_FLEX_ALIGN_CENTER, LV_FLEX_ALIGN_CENTER);
  label(prev, &cv_11b, T->red, "! need");
  label(prev, &cv_11b, T->green, "✓ ready");
  label(prev, &cv_11b, T->yellow, "✻ busy");
  label(prev, &cv_11b, T->muted, "◦ idle");

  lv_obj_t* h1 = label(list, &cv_11, T->muted, "LAYOUT · swipe to cycle");
  lv_obj_set_style_pad_top(h1, 6, 0);
  for (int i = 0; i < N_LAYOUTS; i++) {
    lv_obj_t* r = settingsRow(list, i == curLayout, onPickLayout, i);
    lv_obj_t* l = label(r, &cv_11b, i == curLayout ? T->accent : T->fg);
    lv_label_set_text_fmt(l, "%c · %s", LAYOUTS[i].tag, LAYOUTS[i].name);
    lv_obj_align(l, LV_ALIGN_LEFT_MID, 8, 0);
  }

  lv_obj_t* h2 = label(list, &cv_11, T->muted, "THEME");
  lv_obj_set_style_pad_top(h2, 6, 0);
  for (int i = 0; i < N_THEMES; i++) {
    const Theme& t = THEMES[i];
    lv_obj_t* r = settingsRow(list, i == curTheme, onPickTheme, i);
    const uint32_t sw[] = {t.bg, t.surface, t.fg, t.accent, t.red, t.yellow, t.green};
    for (int k = 0; k < 7; k++) {
      lv_obj_t* s = fill(r, sw[k]);
      lv_obj_set_size(s, 7, 12);
      lv_obj_align(s, LV_ALIGN_LEFT_MID, 7 + k * 7, 0);
    }
    lv_obj_t* l = label(r, &cv_11b, i == curTheme ? T->accent : T->fg, t.name);
    lv_obj_align(l, LV_ALIGN_LEFT_MID, 64, 0);
  }

  if (scroll > 0) {
    lv_obj_update_layout(list);
    lv_obj_scroll_to_y(list, scroll, LV_ANIM_OFF);
  }
}

void openSettings() { buildSettings(); }

// ---------------------------------------------------------------- config + gestures

void applyConfig(int layout, int theme, bool announce) {
  if (layout < 0 || layout >= N_LAYOUTS) layout = curLayout;
  if (theme < 0 || theme >= N_THEMES) theme = curTheme;
  bool themeChanged = theme != curTheme;
  if (layout == curLayout && !themeChanged) return;
  curLayout = layout;
  curTheme = theme;
  T = &THEMES[theme];
  lv_obj_set_style_bg_color(lv_screen_active(), C(T->bg), 0);
  if (themeChanged) {
    bool waiting = waitBox && !lv_obj_has_flag(waitBox, LV_OBJ_FLAG_HIDDEN);
    buildWait();
    if (waiting) lv_obj_remove_flag(waitBox, LV_OBJ_FLAG_HIDDEN);
  }
  rebuild();
  if (settingsObj) buildSettings();
  saveConfig();
  if (announce) sendHello();  // the daemon learns the new layout/theme from hello
}

static void onGesture(lv_event_t*) {
  lv_dir_t dir = lv_indev_get_gesture_dir(lv_indev_active());
  if (dir != LV_DIR_LEFT && dir != LV_DIR_RIGHT) return;
  gestured = true;
  if (settingsObj) return;
  int next = (curLayout + (dir == LV_DIR_LEFT ? 1 : N_LAYOUTS - 1)) % N_LAYOUTS;
  applyConfig(next, curTheme, true);
  char text[48];
  snprintf(text, sizeof text, "%c · %s", LAYOUTS[next].tag, LAYOUTS[next].name);
  showToast(text, T->accent);
}

// ---------------------------------------------------------------- init

void uiInit() {
  lv_obj_t* scr = lv_screen_active();
  lv_obj_remove_flag(scr, LV_OBJ_FLAG_SCROLLABLE);
  lv_obj_set_style_bg_color(scr, C(T->bg), 0);
  lv_obj_set_style_bg_opa(scr, LV_OPA_COVER, 0);
  lv_obj_add_event_cb(scr, onGesture, LV_EVENT_GESTURE, nullptr);

  root = fill(scr, T->bg);
  lv_obj_set_size(root, W, H);

  buildWait();

  toast = fill(lv_layer_top(), T->bg);
  lv_obj_set_size(toast, W - 28, LV_SIZE_CONTENT);
  lv_obj_align(toast, LV_ALIGN_BOTTOM_MID, 0, -14);
  lv_obj_set_style_border_width(toast, 2, 0);
  lv_obj_set_style_pad_hor(toast, 10, 0);
  lv_obj_set_style_pad_ver(toast, 8, 0);
  toastLabel = label(toast, &cv_11, T->fg);
  lv_obj_set_width(toastLabel, lv_pct(100));
  lv_label_set_long_mode(toastLabel, LV_LABEL_LONG_DOT);
  lv_obj_add_flag(toast, LV_OBJ_FLAG_HIDDEN);
  toastTimer = lv_timer_create(hideToast, 1600, nullptr);
  lv_timer_set_auto_delete(toastTimer, false);
  lv_timer_pause(toastTimer);

  lv_timer_create(onSpin, 120, nullptr);
  lv_timer_create(onBlink, 600, nullptr);
  lv_timer_create(onSecond, 1000, nullptr);

  rebuild();
  tickSecond();
}
