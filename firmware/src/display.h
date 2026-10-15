// LovyanGFX device definition for the WT32-SC01 Plus (ESP32-S3).
//
// ST7796UI on an 8-bit parallel (i8080) bus driven by the S3's LCD_CAM
// peripheral, FT6336U touch on I2C, PWM backlight on GPIO45. The panel is
// 320x480 natively; setRotation(3) in main gives the 480x320 landscape the
// dashboard is laid out for, flipped 180 from the panel's native orientation.
//
// The original WT32-SC01 puts the same panel on SPI with a completely
// different pinout — do not mix the two configurations up.
#pragma once

#ifndef LGFX_USE_V1
#define LGFX_USE_V1
#endif
#include <LovyanGFX.hpp>

class LGFX : public lgfx::LGFX_Device {
	lgfx::Panel_ST7796 _panel;
	lgfx::Bus_Parallel8 _bus;
	lgfx::Touch_FT5x06 _touch; // FT6336U speaks the FT5x06 protocol
	lgfx::Light_PWM _light;

public:
	LGFX() {
		{
			auto cfg = _bus.config();
			cfg.freq_write = 20000000;
			cfg.pin_wr = 47;
			cfg.pin_rd = -1; // not wired; the panel is write-only here
			cfg.pin_rs = 0;  // RS/DC — also a strapping pin, which is normal on this board
			cfg.pin_d0 = 9;
			cfg.pin_d1 = 46;
			cfg.pin_d2 = 3;
			cfg.pin_d3 = 8;
			cfg.pin_d4 = 18;
			cfg.pin_d5 = 17;
			cfg.pin_d6 = 16;
			cfg.pin_d7 = 15;
			_bus.config(cfg);
			_panel.setBus(&_bus);
		}
		{
			auto cfg = _panel.config();
			cfg.pin_cs = -1; // tied low on the board
			cfg.pin_rst = 4;
			cfg.pin_busy = -1;
			cfg.panel_width = 320;
			cfg.panel_height = 480;
			cfg.offset_x = 0;
			cfg.offset_y = 0;
			cfg.offset_rotation = 0;
			cfg.dummy_read_pixel = 8;
			cfg.dummy_read_bits = 1;
			cfg.readable = false;
			cfg.invert = false;
			// The panel is BGR: without this an orange sun renders blue.
			// Whites, greys and greens survive the swap unchanged, so it only
			// shows up on saturated colours.
			cfg.rgb_order = true;
			cfg.dlen_16bit = false;
			cfg.bus_shared = false;
			_panel.config(cfg);
		}
		{
			auto cfg = _light.config();
			cfg.pin_bl = 45;
			cfg.invert = false;
			cfg.freq = 44100;
			cfg.pwm_channel = 7;
			_light.config(cfg);
			_panel.setLight(&_light);
		}
		{
			auto cfg = _touch.config();
			cfg.x_min = 0;
			cfg.x_max = 319;
			cfg.y_min = 0;
			cfg.y_max = 479;
			// Polling rather than the INT line on GPIO7: the dashboard reads
			// touch at ~20Hz anyway, and polling cannot miss an edge.
			cfg.pin_int = -1;
			cfg.bus_shared = false;
			cfg.offset_rotation = 0;
			cfg.i2c_port = 1;
			cfg.i2c_addr = 0x38;
			cfg.pin_sda = 6;
			cfg.pin_scl = 5;
			cfg.freq = 400000;
			_touch.config(cfg);
			_panel.setTouch(&_touch);
		}
		setPanel(&_panel);
	}
};
