// ESP32-2432S024R: 2.4" TFT and XPT2046 touch share the HSPI bus.
// Verified on the unit: the controller's memory is 320×240 (landscape) with BGR colour order;
// rotation 5 shows it upright as 240×320 portrait with the USB port at the top. Rotation 7 (the
// same turned by 180°) garbles the picture on this panel, so "USB at the bottom" stays in rotation 5
// and turns pixels and touch in software (main.cpp).
#pragma once

#define LGFX_USE_V1
#include <LovyanGFX.hpp>

static constexpr int BOARD_ROTATION = 5;

// Rough raw XPT2046 range; replaced by the calibration stored in NVS (see main.cpp).
#define CV_TOUCH_X_MIN 300
#define CV_TOUCH_X_MAX 3800
#define CV_TOUCH_Y_MIN 250
#define CV_TOUCH_Y_MAX 3800

class Board : public lgfx::LGFX_Device {
  lgfx::Panel_ILI9341 panel_;
  lgfx::Bus_SPI bus_;
  lgfx::Light_PWM light_;
  lgfx::Touch_XPT2046 touch_;

 public:
  Board() {
    {
      auto c = bus_.config();
      c.spi_host = HSPI_HOST;
      c.spi_mode = 0;
      c.freq_write = 40000000;
      c.freq_read = 16000000;
      c.spi_3wire = false;
      c.use_lock = true;
      c.dma_channel = SPI_DMA_CH_AUTO;
      c.pin_sclk = 14;
      c.pin_mosi = 13;
      c.pin_miso = 12;
      c.pin_dc = 2;
      bus_.config(c);
      panel_.setBus(&bus_);
    }
    {
      auto c = panel_.config();
      c.pin_cs = 15;
      c.pin_rst = -1;  // tied to EN
      c.pin_busy = -1;
      c.panel_width = 320;
      c.panel_height = 240;
      c.offset_rotation = 0;
      c.readable = true;
      c.invert = false;
      c.rgb_order = true;
      c.dlen_16bit = false;
      c.bus_shared = true;
      panel_.config(c);
    }
    {
      auto c = light_.config();
      c.pin_bl = 27;
      c.invert = false;
      c.freq = 12000;
      c.pwm_channel = 7;
      light_.config(c);
      panel_.setLight(&light_);
    }
    {
      auto c = touch_.config();
      c.x_min = CV_TOUCH_X_MIN;
      c.x_max = CV_TOUCH_X_MAX;
      c.y_min = CV_TOUCH_Y_MIN;
      c.y_max = CV_TOUCH_Y_MAX;
      c.pin_int = 36;
      c.bus_shared = true;
      c.offset_rotation = 0;
      c.spi_host = HSPI_HOST;
      c.freq = 1000000;
      c.pin_sclk = 14;
      c.pin_mosi = 13;
      c.pin_miso = 12;
      c.pin_cs = 33;
      touch_.config(c);
      panel_.setTouch(&touch_);
    }
    setPanel(&panel_);
  }
};
