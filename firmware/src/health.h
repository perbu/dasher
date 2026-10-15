// Keeping an unattended board alive: heap telemetry on Serial, a loop
// watchdog, and a reboot when payloads stop arriving.
#pragma once

#include <Arduino.h>

// Logs why the board last reset (including a stall reboot's own note, which
// survives in RTC memory) and arms the loop watchdog. Call early in setup().
void health_begin();

// Heartbeat: logs the heap once a minute and reboots once no payload has
// arrived for kStallMs. last_message_ms is NetStatus::last_message_ms, 0
// until the first payload. Call from loop().
void health_loop(uint32_t last_message_ms);

// One-line heap snapshot tagged with where it was taken.
void health_log(const char *where);
