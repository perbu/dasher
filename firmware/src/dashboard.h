// Parsed form of the JSON document dasher publishes to dashboard/data.
#pragma once

#include <Arduino.h>
#include <time.h>

// dasher publishes at most 3 hours of forecast (internal/fetchers/yr.go);
// the meeting card took the other half of the weather strip.
static const size_t kMaxHours = 3;

// Exactly as many notifications as the detail view has room to list at 34px
// per two-line row. The payload can carry up to 50; keeping them all would
// cost RAM for rows nobody sees. Raising this without changing the row
// spacing in ui_render_detail pushes the last entry off the bottom.
static const size_t kMaxNotifications = 7;

struct Notification {
	char repo[40];   // "owner/name", truncated if longer
	char reason[20]; // "mention", "review_requested", ...
	char type[20];   // "PullRequest", "Issue", ...
	time_t updated_at;
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

	// Absent from the payload until the presence check's first ping; home
	// defaults true so a dasher without the check never blanks the screen.
	bool have_presence = false;
	bool home = true;

	// Next calendar event. have_calendar means dasher's calendar fetcher has
	// delivered; meeting_start == 0 then means "no upcoming event".
	bool have_calendar = false;
	char meeting_title[64];
	time_t meeting_start = 0;
	time_t meeting_end = 0;
	bool meeting_all_day = false;
};

// Parses a published payload into out. Returns false and leaves out untouched
// if the document does not deserialize; missing sections just clear their
// have_* flag, since dasher omits a source until its first successful fetch.
bool dashboard_parse(const char *json, size_t len, Dashboard &out);
