#include "net.h"

#include <PubSubClient.h>
#include <WiFi.h>
#include <WiFiClientSecure.h>
#include <time.h>

#include "certs.h"
#include "config.h"

// dasher publishes the whole notification list, and the GitHub API pages at
// 50, so a busy inbox reaches a few KB. PubSubClient silently drops anything
// larger than its buffer, so size for the worst case rather than the common one.
static const uint16_t kMqttBuffer = 16384;

static WiFiClientSecure tls;
static PubSubClient mqtt(tls);
static NetStatus status;
static PayloadHandler payload_handler = nullptr;
static ProgressHandler progress_handler = nullptr;

static uint32_t next_attempt_ms = 0;
static uint32_t backoff_ms = 2000;

static void progress(const char *headline, const char *detail) {
	Serial.printf("[net] %s%s%s\n", headline, detail[0] ? " - " : "", detail);
	if (progress_handler != nullptr) {
		progress_handler(headline, detail);
	}
}

static void on_message(char *topic, uint8_t *payload, unsigned int length) {
	(void)topic;
	status.last_message_ms = millis();
	if (payload_handler != nullptr) {
		payload_handler((const char *)payload, length);
	}
}

static void connect_wifi() {
	WiFi.mode(WIFI_STA);
	WiFi.setSleep(false); // modem sleep adds seconds of latency to a retained push
	WiFi.setAutoReconnect(true);
	WiFi.begin(WIFI_SSID, WIFI_PASS);

	char detail[48];
	snprintf(detail, sizeof(detail), "%s", WIFI_SSID);
	progress("connecting to wifi", detail);
	for (int i = 0; i < 60 && WiFi.status() != WL_CONNECTED; i++) {
		delay(250);
	}
	status.wifi_up = WiFi.status() == WL_CONNECTED;
	if (status.wifi_up) {
		snprintf(detail, sizeof(detail), "%s", WiFi.localIP().toString().c_str());
		progress("wifi connected", detail);
	}
}

// TLS verifies the broker certificate's validity window, and the ESP32 boots
// at the epoch — so the clock has to be right before the handshake, not after.
static void sync_clock() {
	configTzTime(TZ_SPEC, NTP_SERVER_1, NTP_SERVER_2);
	progress("syncing clock", NTP_SERVER_1);
	for (int i = 0; i < 40 && time(nullptr) < 1700000000; i++) {
		delay(250);
	}
	status.time_ok = time(nullptr) >= 1700000000;
	if (status.time_ok) {
		char now[32];
		time_t t = time(nullptr);
		struct tm lt;
		localtime_r(&t, &lt);
		strftime(now, sizeof(now), "%Y-%m-%d %H:%M:%S", &lt);
		progress("clock synced", now);
	}
}

static bool connect_mqtt() {
	char id[48];
	snprintf(id, sizeof(id), "%s-%04x", MQTT_CLIENT_ID, (uint16_t)(ESP.getEfuseMac() & 0xffff));

	char detail[48];
	snprintf(detail, sizeof(detail), "%s:%d", MQTT_HOST, MQTT_PORT);
	progress("connecting to broker", detail);

	if (mqtt.connect(id)) {
		// Retained, QoS 1: the current payload lands the moment we subscribe.
		mqtt.subscribe(MQTT_TOPIC, 1);
		status.mqtt_up = true;
		status.detail[0] = '\0';
		progress("subscribed", MQTT_TOPIC);
		return true;
	}

	// PubSubClient's rc is generic; the TLS layer knows why the socket died.
	char err[80] = {0};
	int tls_err = tls.lastError(err, sizeof(err));
	snprintf(status.detail, sizeof(status.detail), "mqtt rc %d", mqtt.state());
	Serial.printf("[net] connect failed: rc=%d tls=%d %s\n", mqtt.state(), tls_err, err);
	status.mqtt_up = false;
	return false;
}

void net_begin(PayloadHandler on_payload, ProgressHandler on_progress) {
	payload_handler = on_payload;
	progress_handler = on_progress;

	// The device proves itself with its own certificate, and only trusts the
	// dasher CA — the broker requires mutual TLS in both directions.
	tls.setCACert(kMqttCaCert);
	tls.setCertificate(kMqttClientCert);
	tls.setPrivateKey(kMqttClientKey);

	mqtt.setServer(MQTT_HOST, MQTT_PORT);
	mqtt.setCallback(on_message);
	mqtt.setBufferSize(kMqttBuffer);
	mqtt.setKeepAlive(60);
	// A retained push arrives whole; the default 15s is tight for a slow AP.
	mqtt.setSocketTimeout(30);

	connect_wifi();
	if (status.wifi_up) {
		sync_clock();
	}
	if (status.wifi_up && status.time_ok) {
		connect_mqtt();
	}
}

void net_loop() {
	status.wifi_up = WiFi.status() == WL_CONNECTED;
	status.rssi = status.wifi_up ? WiFi.RSSI() : 0;

	if (!status.wifi_up) {
		status.mqtt_up = false;
		snprintf(status.detail, sizeof(status.detail), "wifi down");
		// WiFi.setAutoReconnect handles the retry; nothing else can proceed.
		return;
	}
	if (!status.time_ok) {
		status.time_ok = time(nullptr) >= 1700000000;
		if (!status.time_ok) {
			return;
		}
	}

	if (mqtt.connected()) {
		status.mqtt_up = true;
		mqtt.loop();
		return;
	}

	status.mqtt_up = false;
	const uint32_t now = millis();
	if ((int32_t)(now - next_attempt_ms) < 0) {
		return;
	}
	if (connect_mqtt()) {
		backoff_ms = 2000;
	} else {
		// Back off to 30s: a broker that is down tends to stay down for a
		// while, and each failed handshake costs a couple of seconds.
		backoff_ms = backoff_ms >= 30000 ? 30000 : backoff_ms * 2;
	}
	next_attempt_ms = millis() + backoff_ms;
}

const NetStatus &net_status() { return status; }
