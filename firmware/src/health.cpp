#include "health.h"

#include <esp_attr.h>
#include <esp_heap_caps.h>
#include <esp_system.h>
#include <esp_task_wdt.h>

// dasher heartbeats every 30s (publish.interval), so ten minutes of silence
// is twenty missed payloads: not a slow server, a wedged link. A reboot costs
// the last frame, which by then is stale anyway.
static const uint32_t kStallMs = 10 * 60 * 1000;

static const uint32_t kLogEveryMs = 60 * 1000;

// Longer than the worst blocking path in net.cpp: a 30s TCP connect, the 30s
// handshake timeout and PubSubClient's 30s wait for CONNACK.
static const uint32_t kWatchdogS = 180;

// Written just before a deliberate reboot and read back on the next boot.
// RTC_NOINIT memory survives esp_restart() and panics, not a power cycle.
struct RebootNote {
	uint32_t magic;
	uint32_t uptime_s;
	uint32_t internal_free;
	uint32_t internal_largest;
	uint32_t internal_min;
	char why[24];
};
static const uint32_t kNoteMagic = 0xda5b007;
static RTC_NOINIT_ATTR RebootNote note;

static uint32_t last_log_ms = 0;

static const char *reset_reason() {
	switch (esp_reset_reason()) {
	case ESP_RST_POWERON: return "power on";
	case ESP_RST_SW: return "software";
	case ESP_RST_PANIC: return "panic";
	case ESP_RST_INT_WDT: return "interrupt watchdog";
	case ESP_RST_TASK_WDT: return "task watchdog";
	case ESP_RST_WDT: return "watchdog";
	case ESP_RST_BROWNOUT: return "brownout";
	case ESP_RST_DEEPSLEEP: return "deep sleep";
	default: return "other";
	}
}

void health_log(const char *where) {
	// Internal RAM is what runs out: mbedTLS allocates all of its handshake
	// buffers there, and needs them in large contiguous blocks. The largest
	// free block is the number to watch, not the total.
	Serial.printf("[heap] %-14s int free=%u largest=%u min=%u  psram free=%u  up=%lus\n",
	              where,
	              (unsigned)heap_caps_get_free_size(MALLOC_CAP_INTERNAL),
	              (unsigned)heap_caps_get_largest_free_block(MALLOC_CAP_INTERNAL),
	              (unsigned)heap_caps_get_minimum_free_size(MALLOC_CAP_INTERNAL),
	              (unsigned)heap_caps_get_free_size(MALLOC_CAP_SPIRAM),
	              (unsigned long)(millis() / 1000));
}

void health_begin() {
	Serial.printf("[health] reset reason: %s\n", reset_reason());
	if (note.magic == kNoteMagic) {
		Serial.printf("[health] last reboot: %s after %lus, int free=%u largest=%u min=%u\n",
		              note.why, (unsigned long)note.uptime_s, (unsigned)note.internal_free,
		              (unsigned)note.internal_largest, (unsigned)note.internal_min);
	}
	note.magic = 0;

	// The core feeds the watchdog before every loop() call once the loop
	// task is subscribed; this only stretches the 5s default past net.cpp's
	// blocking connects. A hang inside them is the case the stall check in
	// health_loop can never catch.
	esp_task_wdt_init(kWatchdogS, true);
	enableLoopWDT();

	health_log("boot");
}

static void reboot(const char *why) {
	note.magic = kNoteMagic;
	note.uptime_s = millis() / 1000;
	note.internal_free = heap_caps_get_free_size(MALLOC_CAP_INTERNAL);
	note.internal_largest = heap_caps_get_largest_free_block(MALLOC_CAP_INTERNAL);
	note.internal_min = heap_caps_get_minimum_free_size(MALLOC_CAP_INTERNAL);
	snprintf(note.why, sizeof(note.why), "%s", why);

	Serial.printf("[health] rebooting: %s\n", why);
	health_log("reboot");
	Serial.flush();
	delay(100);
	esp_restart();
}

void health_loop(uint32_t last_message_ms) {
	const uint32_t now = millis();
	if (now - last_log_ms >= kLogEveryMs) {
		health_log("tick");
		last_log_ms = now;
	}

	// Measured from boot until the first payload, so a board that never gets
	// one is caught too.
	if (now - last_message_ms >= kStallMs) {
		reboot("no data");
	}
}
