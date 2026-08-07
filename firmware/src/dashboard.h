// Parsed form of the JSON document dasher publishes to dashboard/data.
#pragma once

#include <Arduino.h>
#include <time.h>

// dasher publishes at most 6 hours of forecast (internal/fetchers/yr.go).
static const size_t kMaxHours = 6;

// Only as many notifications as the detail view has room to list. The payload
// can carry up to 50; keeping them all would cost RAM for rows nobody sees.
static const size_t kMaxNotifications = 6;

struct Notification {
	char repo[40];   // "owner/name", truncated if longer
	char reason[20]; // "mention", "review_requested", ...
	char type[20];   // "PullRequest", "Issue", ...
};

struct HourForecast {
	time_t at;
	float temperature_c;
	float precipitation_mm;
	char symbol[28]; // met.no symbol_code, e.g. "lightrainshowers_day"
};

struct Dashboard {
	time_t updated_at = 0;
	uint32_t received_ms = 0; // millis() when the payload was parsed

	bool have_github = false;
	int unread = 0;
	int repos = 0; // distinct repositories among all the notifications
	Notification notifications[kMaxNotifications];
	uint8_t notification_count = 0; // how many of the above are filled, newest first

	bool have_claude = false;
	float five_hour_pct = 0;
	time_t five_hour_resets_at = 0;
	float seven_day_pct = 0;
	time_t seven_day_resets_at = 0;

	bool have_weather = false;
	HourForecast hours[kMaxHours];
	uint8_t hour_count = 0;
};

// Parses a published payload into out. Returns false and leaves out untouched
// if the document does not deserialize; missing sections just clear their
// have_* flag, since dasher omits a source until its first successful fetch.
bool dashboard_parse(const char *json, size_t len, Dashboard &out);
