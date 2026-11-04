// The five layouts from design/index.html, rebuilt from scratch whenever what they show changes.
#include "app.h"

static const char* STATE_LABEL[4] = {"NEEDS YOU", "READY", "WORKING", "IDLE"};
static const char* SUM_GLYPH[4] = {"!", "✓", "✻", "◦"};

// width of a label's text in pixels
static int textWidth(lv_obj_t* l) {
  lv_obj_update_layout(l);
  return lv_obj_get_width(l);
}

static void hline(lv_obj_t* o, lv_border_side_t side, uint32_t color) {
  lv_obj_set_style_border_side(o, side, 0);
  lv_obj_set_style_border_width(o, 1, 0);
  lv_obj_set_style_border_color(o, C(color), 0);
}

// ================================================================ A · waybar

static void buildWaybar(lv_obj_t* root) {
  constexpr int BAR_H = 20, FOOT_H = 34, ROW_H = 44, SIDE_W = 44, MX = 11, MW = W - MX - 8 - SIDE_W;
  int count[4];
  countStates(count);

  lv_obj_t* bar = fill(root, T->bg2);
  lv_obj_set_size(bar, W, BAR_H);
  hline(bar, LV_BORDER_SIDE_BOTTOM, T->border);
  headerTarget(bar);
  int x = 2;
  for (int i = 0; i < nSessions && i < 9; i++) {
    State st = sessions[i].state;
    lv_obj_t* l = label(bar, &cv_11b, st == S_INPUT ? T->bg : stateColor(st));
    lv_label_set_text_fmt(l, "%d", i + 1);
    lv_obj_set_size(l, 14, BAR_H - 1);
    lv_obj_set_style_pad_top(l, 3, 0);
    lv_obj_set_style_text_align(l, LV_TEXT_ALIGN_CENTER, 0);
    if (st == S_INPUT) {
      lv_obj_set_style_bg_color(l, C(T->red), 0);
      lv_obj_set_style_bg_opa(l, LV_OPA_COVER, 0);
    }
    lv_obj_set_pos(l, x, 0);
    tapTarget(l, i);
    lv_obj_add_flag(l, LV_OBJ_FLAG_EVENT_BUBBLE);  // long-press still reaches the bar
    x += 14;
  }
  if (!nSessions) lv_obj_set_pos(label(bar, &cv_11b, T->muted, "–"), 6, 3);
  lv_obj_align(clockLabel(bar, &cv_11, T->fg), LV_ALIGN_CENTER, 0, 0);
  lv_obj_t* cnt = label(bar, &cv_11, T->muted);
  lv_label_set_text_fmt(cnt, "◉ %d", nSessions);
  lv_obj_align(cnt, LV_ALIGN_RIGHT_MID, -4, 0);

  lv_obj_t* list = scroller(root);
  lv_obj_set_size(list, W, H - BAR_H - FOOT_H);
  lv_obj_set_pos(list, 0, BAR_H);
  for (int i = 0; i < nSessions; i++) {
    const Session& s = sessions[i];
    lv_obj_t* row = box(list);
    lv_obj_set_size(row, W, ROW_H);
    hline(row, LV_BORDER_SIDE_BOTTOM, T->surface);
    tapTarget(row, i);

    lv_obj_set_size(fill(row, stateColor(s.state)), 3, ROW_H - 1);

    lv_obj_t* name = label(row, &cv_13b, T->fg, s.name);
    lv_obj_set_pos(name, MX, 6);
    maxLine(name, MW);
    int nw = textWidth(name);
    if (s.branch[0] && MW - nw - 6 > 16) {
      lv_obj_t* br = label(row, &cv_11, T->muted, s.branch);
      maxLine(br, MW - nw - 6);
      lv_obj_set_pos(br, MX + nw + 6, 8);
    }
    lv_obj_t* act = label(row, &cv_11, s.state == S_INPUT ? T->red : T->muted, s.activity);
    oneLine(act, MW);
    lv_obj_set_pos(act, MX, 25);

    lv_obj_align(glyphLabel(row, &cv_14b, i), LV_ALIGN_TOP_RIGHT, -8, 5);
    lv_obj_align(timeLabel(row, &cv_11, T->muted, i), LV_ALIGN_TOP_RIGHT, -8, 25);
  }
  if (!nSessions) emptyState(root);

  lv_obj_t* foot = fill(root, T->bg2);
  lv_obj_set_size(foot, W, FOOT_H);
  lv_obj_set_pos(foot, 0, H - FOOT_H);
  hline(foot, LV_BORDER_SIDE_TOP, T->border);
  lv_obj_t* seg = fill(foot, T->surface);
  lv_obj_set_size(seg, W - 16, 4);
  lv_obj_set_pos(seg, 8, 6);
  const uint32_t segColor[4] = {T->red, T->green, T->yellow, T->muted};
  int parts = 0;
  for (int k = 0; k < 4; k++) parts += count[k] ? 1 : 0;
  int avail = (W - 16) - (parts > 0 ? (parts - 1) * 2 : 0), done = 0, prevEnd = 0;
  x = 0;
  for (int k = 0; k < 4; k++) {
    if (!count[k]) continue;
    done += count[k];
    int end = avail * done / nSessions, w = LV_MAX(1, end - prevEnd);
    prevEnd = end;
    lv_obj_t* p = fill(seg, segColor[k]);
    lv_obj_set_size(p, w, 4);
    lv_obj_set_pos(p, x, 0);
    x += w + 2;
  }
  lv_obj_t* sum = box(foot);
  lv_obj_set_size(sum, W - 16, 14);
  lv_obj_set_pos(sum, 8, 15);
  lv_obj_set_flex_flow(sum, LV_FLEX_FLOW_ROW);
  lv_obj_set_flex_align(sum, LV_FLEX_ALIGN_SPACE_BETWEEN, LV_FLEX_ALIGN_CENTER, LV_FLEX_ALIGN_CENTER);
  for (int k = 0; k < 4; k++) lv_label_set_text_fmt(label(sum, &cv_11b, segColor[k]), "%s %d", SUM_GLYPH[k], count[k]);
  lv_obj_t* more = label(sum, &cv_11, T->muted, " ");
  if (nSessions > 6) lv_label_set_text_fmt(more, "+%d ↓", nSessions - 6);
}

// ================================================================ B · tiling

static void buildTiling(lv_obj_t* root) {
  constexpr int BAR_H = 18, GAP = 5;
  int count[4];
  countStates(count);

  lv_obj_t* bar = fill(root, T->bg2);
  lv_obj_set_size(bar, W, BAR_H);
  headerTarget(bar);
  lv_obj_align(label(bar, &cv_11b, T->accent, "cyberviber"), LV_ALIGN_LEFT_MID, 7, 0);
  lv_obj_align(clockLabel(bar, &cv_11, T->fg), LV_ALIGN_CENTER, 0, 0);
  lv_obj_t* total = label(bar, &cv_11, T->muted);
  lv_label_set_text_fmt(total, "%d", nSessions);
  lv_obj_align(total, LV_ALIGN_RIGHT_MID, -7, 0);
  if (count[S_INPUT]) {
    lv_obj_t* in = label(bar, &cv_11b, T->red);
    lv_label_set_text_fmt(in, "%d!", count[S_INPUT]);
    lv_obj_align(in, LV_ALIGN_RIGHT_MID, -7 - textWidth(total) - 6, 0);
  }

  if (!nSessions) {
    emptyState(root);
    return;
  }

  int shown = LV_MIN(nSessions, 9);
  bool overflow = nSessions > 9;
  int cols = shown <= 2 ? 1 : shown <= 6 ? 2 : 3;
  int rows = (shown + cols - 1) / cols, rem = shown % cols;
  int aw = W - 2 * GAP, ah = H - BAR_H - 2 * GAP;
  int tw = (aw - (cols - 1) * GAP) / cols, th = (ah - (rows - 1) * GAP) / rows;

  int focus = 0;
  for (int i = 0; i < nSessions; i++)
    if (!strcmp(sessions[i].id, focusId)) focus = i;

  for (int i = 0; i < shown; i++) {
    int x = GAP + (i % cols) * (tw + GAP), y = BAR_H + GAP + (i / cols) * (th + GAP);
    int w = tw;
    if (i == shown - 1 && rem) w = tw * (cols - rem + 1) + GAP * (cols - rem);

    if (overflow && i == shown - 1) {  // 9th tile: "+N more"
      lv_obj_t* t = frame(root, T->border, 2);
      lv_obj_set_pos(t, x, y);
      lv_obj_set_size(t, w, th);
      lv_obj_t* l = label(t, &cv_13b, T->muted);
      lv_label_set_text_fmt(l, "+%d", nSessions - shown + 1);
      lv_obj_center(l);
      break;
    }

    const Session& s = sessions[i];
    uint32_t bc = s.state == S_INPUT ? T->red : i == focus ? T->accent : T->border;
    lv_obj_t* t = frame(root, bc, 2);
    lv_obj_set_pos(t, x, y);
    lv_obj_set_size(t, w, th);
    lv_obj_set_style_pad_hor(t, 6, 0);
    lv_obj_set_style_pad_ver(t, 4, 0);
    tapTarget(t, i);
    if (s.state == S_INPUT) blinkBorder(t, T->red, T->border);

    bool narrow = w <= 84, big = th >= 140, low = th <= 84;
    int cw = w - 4 - 12;

    lv_obj_t* name = label(t, &cv_11b, T->fg, s.name);
    oneLine(name, narrow ? cw : cw - 40);
    if (!narrow) lv_obj_align(timeLabel(t, &cv_11, T->muted, i), LV_ALIGN_TOP_RIGHT, 0, 0);

    const lv_font_t* gf = big ? &cv_40 : &cv_22b;
    int gy = 17, gh = big ? 44 : 24;
    lv_obj_t* g = glyphLabel(t, gf, i);
    lv_obj_set_pos(g, 0, gy);
    if (!narrow) {
      lv_obj_t* lab = label(t, &cv_11b, stateColor(s.state), STATE_LABEL[s.state]);
      lv_obj_set_style_text_letter_space(lab, 1, 0);
      lv_obj_set_pos(lab, (big ? 40 : 22) + 6, gy + (gh - 14) / 2);
    }

    if (!low) {
      int lines = big ? 3 : 2;
      lv_obj_t* act = label(t, &cv_11, s.state == S_INPUT ? T->fg : T->muted, s.activity);
      lv_label_set_long_mode(act, LV_LABEL_LONG_DOT);
      lv_obj_set_size(act, cw, lines * 14);
      lv_obj_set_pos(act, 0, gy + gh + 4);
    }
    if (!narrow) {
      char where[64], foot[112];
      fmtWhere(where, sizeof where, s);
      if (s.branch[0]) snprintf(foot, sizeof foot, "%s · %s", where, s.branch);
      else snprintf(foot, sizeof foot, "%s", where);
      lv_obj_t* f = label(t, &cv_11, T->muted, foot);
      oneLine(f, cw);
      lv_obj_align(f, LV_ALIGN_BOTTOM_LEFT, 0, 0);
    }
  }
}

// ================================================================ C · btop

// a rounded box with titles sitting on its border
static lv_obj_t* btopBox(lv_obj_t* root, int x, int y, int w, int h) {
  lv_obj_t* b = frame(root, T->border, 1);
  lv_obj_set_style_radius(b, 6, 0);
  lv_obj_set_pos(b, x, y);
  lv_obj_set_size(b, w, h);
  return b;
}

static lv_obj_t* btopTitle(lv_obj_t* root, const char* sup, const char* text, uint32_t color, lv_align_t align, int x, int y) {
  lv_obj_t* t = fill(root, T->bg);
  lv_obj_set_size(t, LV_SIZE_CONTENT, LV_SIZE_CONTENT);
  lv_obj_set_style_pad_hor(t, 3, 0);
  lv_obj_set_flex_flow(t, LV_FLEX_FLOW_ROW);
  if (sup) label(t, &cv_11, T->accent, sup);
  label(t, sup ? &cv_11b : &cv_11, color, text);
  lv_obj_align(t, align, x, y);
  return t;
}

static void buildBtop(lv_obj_t* root) {
  int count[4];
  countStates(count);
  int n = LV_MAX(1, nSessions);

  lv_obj_t* b1 = btopBox(root, 4, 8, W - 8, 80);
  headerTarget(b1);
  btopTitle(root, "¹", "status", T->fg, LV_ALIGN_TOP_LEFT, 12, 1);
  lv_obj_t* ct = fill(root, T->bg);
  lv_obj_set_size(ct, LV_SIZE_CONTENT, LV_SIZE_CONTENT);
  lv_obj_set_style_pad_hor(ct, 3, 0);
  clockLabel(ct, &cv_11, T->muted);
  lv_obj_align(ct, LV_ALIGN_TOP_RIGHT, -12, 1);

  static const char* MLAB[4] = {"need", "ready", "busy", "idle"};
  const uint32_t mcol[4] = {T->red, T->green, T->yellow, T->muted};
  for (int k = 0; k < 4; k++) {
    int y = 8 + 12 + k * 15;
    lv_obj_set_pos(label(root, &cv_11, T->muted, MLAB[k]), 12, y);
    int f = (count[k] * 14 + n / 2) / n;
    for (int c = 0; c < 14; c++) {
      lv_obj_t* cell = fill(root, c < f ? mcol[k] : T->surface);
      lv_obj_set_size(cell, 8, 7);
      lv_obj_set_pos(cell, 52 + c * 9, y + 4);
    }
    lv_obj_t* v = label(root, &cv_11b, mcol[k]);
    lv_label_set_text_fmt(v, "%d", count[k]);
    lv_obj_align(v, LV_ALIGN_TOP_RIGHT, -12, y);
  }

  constexpr int B2Y = 98, B2H = H - 98 - 8;
  btopBox(root, 4, B2Y, W - 8, B2H);
  btopTitle(root, "²", "sessions", T->fg, LV_ALIGN_TOP_LEFT, 12, B2Y - 7);
  char buf[32];
  snprintf(buf, sizeof buf, "%d total", nSessions);
  btopTitle(root, nullptr, buf, T->muted, LV_ALIGN_TOP_RIGHT, -12, B2Y - 7);
  if (nSessions > 8) snprintf(buf, sizeof buf, "↓ %d more", nSessions - 8);
  else snprintf(buf, sizeof buf, "tap → focus");
  btopTitle(root, nullptr, buf, T->muted, LV_ALIGN_TOP_LEFT, 12, B2Y + B2H - 8);

  // columns inside the box (x from the box's inner left edge at 5)
  constexpr int IX = 5, IW = W - 10, CG = 6, CN = 24, CC = 150, NW = CC - 4 - CN;
  lv_obj_set_pos(label(root, &cv_11, T->muted, "NAME"), IX + CN, B2Y + 6);
  lv_obj_set_pos(label(root, &cv_11, T->muted, "CTX"), IX + CC, B2Y + 6);
  lv_obj_t* th = label(root, &cv_11, T->muted, "TIME");
  lv_obj_align(th, LV_ALIGN_TOP_RIGHT, -(W - IX - IW) - 6, B2Y + 6);

  lv_obj_t* list = scroller(root);
  lv_obj_set_pos(list, IX, B2Y + 23);
  lv_obj_set_size(list, IW, B2H - 23 - 9);
  for (int i = 0; i < nSessions; i++) {
    const Session& s = sessions[i];
    lv_obj_t* row = s.state == S_INPUT ? fill(list, T->surface) : box(list);
    lv_obj_set_size(row, IW, 22);
    tapTarget(row, i);
    lv_obj_t* g = glyphLabel(row, &cv_11b, i);
    lv_obj_set_width(g, 14);
    lv_obj_set_style_text_align(g, LV_TEXT_ALIGN_CENTER, 0);
    lv_obj_set_pos(g, CG, 4);
    lv_obj_t* nm = label(row, &cv_11, s.state == S_IDLE ? T->muted : T->fg, s.name);
    oneLine(nm, NW);
    lv_obj_set_pos(nm, CN, 4);
    int f = (s.ctx + 10) / 20;
    uint32_t cc = s.ctx >= 80 ? T->red : s.ctx >= 50 ? T->yellow : T->green;
    for (int c = 0; c < 5; c++) {
      lv_obj_t* cell = fill(row, c < f ? cc : s.state == S_INPUT ? T->border : T->surface);
      lv_obj_set_size(cell, 6, 8);
      lv_obj_set_pos(cell, CC + c * 7, 7);
    }
    lv_obj_align(timeLabel(row, &cv_11, T->muted, i), LV_ALIGN_TOP_RIGHT, -6, 4);
  }
  if (!nSessions) {
    lv_obj_t* e = label(root, &cv_11, T->muted, "no claude running");
    lv_obj_align(e, LV_ALIGN_TOP_MID, 0, B2Y + 50);
  }
}

// ================================================================ D · walker

static void buildWalker(lv_obj_t* root) {
  lv_obj_set_style_bg_color(root, C(T->bg2), 0);
  lv_obj_t* p = fill(root, T->bg);
  lv_obj_set_pos(p, 8, 8);
  lv_obj_set_size(p, W - 16, H - 16);
  lv_obj_set_style_border_width(p, 2, 0);
  lv_obj_set_style_border_color(p, C(T->accent), 0);
  constexpr int PW = W - 16 - 4, PH = H - 16 - 4, IN_H = 34, FT_H = 20;

  lv_obj_t* in = box(p);
  lv_obj_set_size(in, PW, IN_H);
  hline(in, LV_BORDER_SIDE_BOTTOM, T->border);
  headerTarget(in);
  lv_obj_align(label(in, &cv_13b, T->accent, "›"), LV_ALIGN_LEFT_MID, 10, 0);
  lv_obj_t* q = label(in, &cv_13b, T->fg, "claude");
  lv_obj_align(q, LV_ALIGN_LEFT_MID, 24, 0);
  lv_obj_t* caret = label(in, &cv_13b, T->accent, "▏");
  lv_obj_align(caret, LV_ALIGN_LEFT_MID, 24 + textWidth(q), 0);
  blinkOpa(caret);
  lv_obj_t* cnt = label(in, &cv_11, T->muted);
  lv_label_set_text_fmt(cnt, "%d running", nSessions);
  lv_obj_align(cnt, LV_ALIGN_RIGHT_MID, -10, 0);

  lv_obj_t* list = scroller(p);
  lv_obj_set_pos(list, 0, IN_H);
  lv_obj_set_size(list, PW, PH - IN_H - FT_H);
  lv_obj_set_style_pad_all(list, 4, 0);
  constexpr int RW = PW - 8, TX = 45, RT = 68, TW = RW - TX - RT;
  for (int i = 0; i < nSessions; i++) {
    const Session& s = sessions[i];
    lv_obj_t* row = i == 0 ? fill(list, T->surface) : box(list);
    lv_obj_set_size(row, RW, 48);
    tapTarget(row, i);

    lv_obj_t* ic = frame(row, stateColor(s.state), 2);
    lv_obj_set_size(ic, 30, 30);
    lv_obj_set_pos(ic, 6, 9);
    lv_obj_center(glyphLabel(ic, &cv_14b, i));

    lv_obj_t* nm = label(row, &cv_13b, i == 0 ? T->accent : T->fg, s.name);
    oneLine(nm, TW);
    lv_obj_set_pos(nm, TX, 8);
    lv_obj_t* act = label(row, &cv_11, s.state == S_INPUT ? T->red : T->muted, s.activity);
    oneLine(act, TW);
    lv_obj_set_pos(act, TX, 27);

    char where[48];
    fmtWhereShort(where, sizeof where, s);
    lv_obj_t* wl = label(row, &cv_11, T->muted, where);
    maxLine(wl, 56);
    lv_obj_align(wl, LV_ALIGN_TOP_RIGHT, -6, 9);
    lv_obj_align(timeLabel(row, &cv_11, T->muted, i), LV_ALIGN_TOP_RIGHT, -6, 27);
  }
  if (!nSessions) {
    lv_obj_t* e = label(p, &cv_11, T->muted, "no results");
    lv_obj_align(e, LV_ALIGN_TOP_MID, 0, IN_H + 50);
  }

  lv_obj_t* ft = box(p);
  lv_obj_set_size(ft, PW, FT_H);
  lv_obj_set_pos(ft, 0, PH - FT_H);
  hline(ft, LV_BORDER_SIDE_TOP, T->border);
  lv_obj_align(label(ft, &cv_11, T->muted, "tap to focus terminal"), LV_ALIGN_LEFT_MID, 10, 0);
  if (nSessions > 5) {
    lv_obj_t* m = label(ft, &cv_11, T->muted);
    lv_label_set_text_fmt(m, "+%d ↓", nSessions - 5);
    lv_obj_align(m, LV_ALIGN_RIGHT_MID, -10, 0);
  }
}

// ================================================================ E · hyprlock

static void lockChips(lv_obj_t* root, int from, int max, int y) {
  constexpr int CW = (W - 20 - 5) / 2;
  int rest = nSessions - from, shown = LV_MIN(rest, max);
  for (int k = 0; k < shown + (rest > shown ? 1 : 0); k++) {
    lv_obj_t* c = frame(root, T->border, 1);
    lv_obj_set_size(c, CW, 26);
    lv_obj_set_pos(c, 10 + (k % 2) * (CW + 5), y + (k / 2) * 31);
    if (k == shown) {  // "+N more"
      lv_obj_t* m = label(c, &cv_11, T->muted);
      lv_label_set_text_fmt(m, "+%d more", rest - shown);
      lv_obj_center(m);
      break;
    }
    int i = from + k;
    tapTarget(c, i);
    lv_obj_t* g = glyphLabel(c, &cv_11b, i);
    lv_obj_align(g, LV_ALIGN_LEFT_MID, 7, 0);
    lv_obj_t* nm = label(c, &cv_11, T->fg, sessions[i].name);
    oneLine(nm, CW - 2 - 7 - 14 - 7);
    lv_obj_align(nm, LV_ALIGN_LEFT_MID, 7 + 14, 0);
  }
}

static void buildHyprlock(lv_obj_t* root) {
  int count[4];
  countStates(count);
  bool hot = nSessions > 0 && (sessions[0].state == S_INPUT || sessions[0].state == S_READY);

  if (!hot) {
    lv_obj_t* top = box(root);
    lv_obj_set_size(top, W, 110);
    headerTarget(top);
    lv_obj_t* clk = clockLabel(top, &cv_64b, T->fg);
    lv_obj_set_style_text_letter_space(clk, -3, 0);
    lv_obj_align(clk, LV_ALIGN_TOP_MID, 0, 22);
    lv_obj_align(dateLabel(top, &cv_11, T->muted), LV_ALIGN_TOP_MID, 0, 22 + 64 + 6);

    lv_obj_t* q = frame(root, T->border, 2);
    lv_obj_set_pos(q, 10, 126);
    lv_obj_set_size(q, W - 20, 34);
    lv_obj_t* ql = label(q, &cv_11, T->muted);
    if (nSessions) lv_label_set_text_fmt(ql, "%d working · nothing needs you", count[S_WORKING]);
    else lv_label_set_text(ql, "no claude running");
    lv_obj_center(ql);
    lockChips(root, 0, 4, 170);
    return;
  }

  const Session& s = sessions[0];
  uint32_t sc = stateColor(s.state);

  lv_obj_t* top = box(root);
  lv_obj_set_size(top, W, 24);
  lv_obj_set_flex_flow(top, LV_FLEX_FLOW_ROW);
  lv_obj_set_flex_align(top, LV_FLEX_ALIGN_CENTER, LV_FLEX_ALIGN_CENTER, LV_FLEX_ALIGN_CENTER);
  lv_obj_set_style_pad_column(top, 6, 0);
  headerTarget(top);
  clockLabel(top, &cv_11, T->muted);
  label(top, &cv_11, T->muted, "·");
  dateLabel(top, &cv_11, T->muted);

  lv_obj_t* card = frame(root, sc, 2);
  lv_obj_set_pos(card, 10, 28);
  lv_obj_set_size(card, W - 20, LV_SIZE_CONTENT);
  lv_obj_set_style_pad_all(card, 10, 0);
  lv_obj_set_flex_flow(card, LV_FLEX_FLOW_COLUMN);
  lv_obj_set_style_pad_row(card, 4, 0);
  tapTarget(card, 0);
  constexpr int CW = W - 20 - 4 - 20;

  lv_obj_t* lab = box(card);
  lv_obj_set_size(lab, CW, 14);
  lv_obj_t* dot = label(lab, &cv_11b, sc, "●");
  if (s.state == S_INPUT) blinkOpa(dot);
  lv_obj_t* lt = label(lab, &cv_11b, sc, s.state == S_INPUT ? "NEEDS YOU" : "YOUR TURN");
  lv_obj_set_style_text_letter_space(lt, 2, 0);
  lv_obj_set_pos(lt, 14, 0);
  lv_obj_t* of = label(lab, &cv_11, T->muted);
  lv_label_set_text_fmt(of, "1/%d", count[s.state]);
  lv_obj_align(of, LV_ALIGN_TOP_RIGHT, 0, 0);

  lv_obj_t* nm = label(card, &cv_22b, T->fg, s.name);
  oneLine(nm, CW);
  lv_obj_set_style_pad_top(nm, 4, 0);

  lv_obj_t* act = label(card, &cv_11, T->fg, s.activity);
  lv_label_set_long_mode(act, LV_LABEL_LONG_DOT);
  lv_obj_set_size(act, CW, 3 * 14);

  lv_obj_t* meta = box(card);
  lv_obj_set_size(meta, CW, LV_SIZE_CONTENT);
  lv_obj_set_flex_flow(meta, LV_FLEX_FLOW_ROW);
  lv_obj_set_style_pad_ver(meta, 4, 0);
  char where[64];
  fmtWhere(where, sizeof where, s);
  maxLine(label(meta, &cv_11, T->muted, where), CW - 60);
  label(meta, &cv_11, T->muted, " · ");
  timeLabel(meta, &cv_11, T->muted, 0);

  lv_obj_t* btn = fill(card, sc);
  lv_obj_set_size(btn, CW, 34);
  lv_obj_t* bl = label(btn, &cv_11b, T->bg, "TAP TO JUMP ⟶");
  lv_obj_set_style_text_letter_space(bl, 2, 0);
  lv_obj_center(bl);

  lv_obj_update_layout(card);
  lockChips(root, 1, 6, 28 + lv_obj_get_height(card) + 10);
}

// ================================================================ registry

const Layout LAYOUTS[] = {
    {"waybar", "waybar", 'A', buildWaybar},
    {"tiling", "tiling", 'B', buildTiling},
    {"btop", "btop", 'C', buildBtop},
    {"walker", "walker", 'D', buildWalker},
    {"hyprlock", "hyprlock", 'E', buildHyprlock},
};
const int N_LAYOUTS = sizeof(LAYOUTS) / sizeof(LAYOUTS[0]);
