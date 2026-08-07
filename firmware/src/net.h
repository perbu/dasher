// WiFi, clock sync and the mutual-TLS MQTT subscription.
#pragma once

#include <Arduino.h>

struct NetStatus {
	bool wifi_up = false;
	bool time_ok = false;
	bool mqtt_up = false;
	int rssi = 0;
	uint32_t last_message_ms = 0; // millis() of the last payload received
	char detail[32] = {0};        // last error, shown in the footer
};

// Called with the raw retained payload each time one arrives.
typedef void (*PayloadHandler)(const char *json, size_t len);

// Called with progress messages while bringing the link up, for the boot screen.
typedef void (*ProgressHandler)(const char *headline, const char *detail);

void net_begin(PayloadHandler on_payload, ProgressHandler on_progress);

// Pumps WiFi reconnects, clock sync and the MQTT client. Call from loop().
void net_loop();

const NetStatus &net_status();
