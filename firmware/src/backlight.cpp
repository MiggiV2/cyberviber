// Backlight: user brightness, dimming after dimAfterMin minutes without activity (spec §4.5).
// Activity is a touch or a session that newly needs input; a touch on the dimmed screen
// only wakes it.
#include "app.h"

int brightPct = 100;
int dimAfterMin = 10;

static constexpr uint8_t DIM_LEVEL = 8;  // of 255: readable in a dark room, nearly off otherwise
static constexpr uint32_t FADE_MS = 400;

static uint8_t level = 0;  // what the panel shows right now
static bool dimmed = false;
static uint32_t lastActivity = 0;
static lv_anim_t fade;

static uint8_t fullLevel() { return (uint8_t)LV_CLAMP(DIM_LEVEL, brightPct * 255 / 100, 255); }

static void fadeCb(void*, int32_t v) {
  level = (uint8_t)v;
  setBacklight(level);
}

static void fadeTo(uint8_t target) {
  lv_anim_delete(&fade, fadeCb);
  if (target == level) return;
  lv_anim_init(&fade);
  lv_anim_set_var(&fade, &fade);
  lv_anim_set_exec_cb(&fade, fadeCb);
  lv_anim_set_values(&fade, level, target);
  lv_anim_set_duration(&fade, FADE_MS);
  lv_anim_start(&fade);
}

void backlightInit() {
  level = fullLevel();
  setBacklight(level);
  lastActivity = millis();
}

bool backlightWake() {
  lastActivity = millis();
  if (!dimmed) return false;
  dimmed = false;
  fadeTo(fullLevel());
  return true;
}

void backlightTick() {
  if (dimmed || dimAfterMin <= 0) return;
  if (millis() - lastActivity >= (uint32_t)dimAfterMin * 60000u) {
    dimmed = true;
    fadeTo(LV_MIN(DIM_LEVEL, fullLevel()));
  }
}

// after brightness or dim_after changed: show the new brightness right away
void backlightApply() {
  dimmed = false;
  lastActivity = millis();
  fadeTo(fullLevel());
}
