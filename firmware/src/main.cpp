// dasher dashboard for the WT32-SC01: subscribes to the retained MQTT payload
// and renders it on the 480x320 panel. Tap the screen to dim or brighten.
#include <Arduino.h>
#include <time.h>

#include "config.h"
#include "dashboard.h"
#include "display.h"
#include "net.h"
#include "theme.h"
#include "ui.h"

static LGFX display;
static LGFX_Sprite frame(&display);
static LGFX_Sprite footer(&display);

static Dashboard data;
static bool frame_dirty = true;
static bool bright = true;
static int applied_brightness = -1; // what the backlight was last set to
static uint32_t last_footer_ms = 0;
static int last_minute = -1; // wall-clock minute of the last frame redraw

// Short tap opens or closes the detail view; holding toggles the backlight.
static const uint32_t kLongPressMs = 700;
// The detail view is for answering a question, not for living on. Falling back
// on its own means a tap that is never followed up does not strand the panel.
static const uint32_t kDetailTimeoutMs = 45000;

static bool detail_view = false;
static uint32_t detail_opened_ms = 0;
static bool touch_down = false;
static uint32_t touch_started_ms = 0;
static bool long_press_fired = false;

// The full-frame sprite lives in PSRAM; if it cannot be had, draw straight to
// the panel instead. Tearing beats a blank screen.
static lgfx::LGFXBase *canvas = nullptr;
static lgfx::LGFXBase *footer_canvas = nullptr;

static void push_frame() {
	if (canvas == &frame) {
		frame.pushSprite(0, 0);
	}
}

static void push_footer() {
	if (footer_canvas == &footer) {
		footer.pushSprite(0, kFooterY);
	}
}

static void on_payload(const char *json, size_t len) {
	Serial.printf("[mqtt] payload %u bytes\n", (unsigned)len);
	if (dashboard_parse(json, len, data)) {
		frame_dirty = true;
	}
}

static void on_progress(const char *headline, const char *detail) {
	// Straight to the panel: this runs before the first frame is composed.
	ui_status(display, headline, detail);
}

void setup() {
	Serial.begin(115200);

	// Applied to every localtime_r call; the payload itself is UTC.
	setenv("TZ", TZ_SPEC, 1);
	tzset();

	display.init();
	display.setRotation(3); // 480x320 landscape, flipped 180
	display.setBrightness(BRIGHTNESS_FULL);
	display.fillScreen(kBg);

	frame.setPsram(true);
	frame.setColorDepth(16);
	canvas = frame.createSprite(kScreenW, kScreenH) != nullptr ? (lgfx::LGFXBase *)&frame : (lgfx::LGFXBase *)&display;
	if (canvas == &display) {
		Serial.println("[ui] frame sprite allocation failed, drawing direct");
	}

	footer.setColorDepth(16);
	footer_canvas = footer.createSprite(kScreenW, kFooterH) != nullptr ? (lgfx::LGFXBase *)&footer : (lgfx::LGFXBase *)&display;

	ui_status(display, "starting", "");
	net_begin(on_payload, on_progress);
}

void loop() {
	net_loop();

	const uint32_t now = millis();

	if (detail_view && now - detail_opened_ms >= kDetailTimeoutMs) {
		detail_view = false;
		frame_dirty = true;
	}

	// Presence wins over the manual toggle: nobody home, backlight off. The
	// bright/dim choice survives and is restored when the host comes back.
	const uint8_t want = (!data.have_presence || data.home) ? (bright ? BRIGHTNESS_FULL : BRIGHTNESS_DIM) : 0;
	if (want != applied_brightness) {
		display.setBrightness(want);
		applied_brightness = want;
	}

	// The meeting countdown ages between payloads, which arrive minutes
	// apart: redraw the frame when the wall-clock minute rolls over.
	if (!detail_view && data.meeting_title[0] != '\0') {
		const time_t t = time(nullptr);
		struct tm lt;
		localtime_r(&t, &lt);
		const int minute = lt.tm_hour * 60 + lt.tm_min;
		if (minute != last_minute) {
			last_minute = minute;
			frame_dirty = true;
		}
	}

	if (frame_dirty) {
		if (detail_view) {
			// Covers the whole panel, footer band included.
			ui_render_detail(*canvas, data);
		} else {
			ui_render(*canvas, data);
		}
		push_frame();
		// The footer sits inside the frame, so redraw it in the same breath.
		last_footer_ms = 0;
		frame_dirty = false;
	}

	if (!detail_view && now - last_footer_ms >= 1000) {
		if (footer_canvas == &footer) {
			ui_render_footer(footer, data, net_status(), 0);
			push_footer();
		} else {
			ui_render_footer(display, data, net_status(), kFooterY);
		}
		last_footer_ms = now;
	}

	// Short tap toggles the detail view, long press the backlight. The long
	// press fires while the finger is still down, so it announces itself by
	// the screen changing rather than leaving you guessing how long to hold.
	int32_t tx, ty;
	const bool pressed = display.getTouch(&tx, &ty);
	if (pressed && !touch_down) {
		touch_started_ms = now;
		long_press_fired = false;
	} else if (pressed && !long_press_fired && now - touch_started_ms >= kLongPressMs) {
		bright = !bright;
		long_press_fired = true;
	} else if (!pressed && touch_down && !long_press_fired) {
		detail_view = !detail_view;
		detail_opened_ms = now;
		frame_dirty = true;
	}
	touch_down = pressed;

	delay(20);
}
