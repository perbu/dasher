#include "dashboard.h"

#include <ArduinoJson.h>
#include <string.h>

// Days since the Unix epoch for a civil date (Howard Hinnant's algorithm).
// timegm() is not dependable across newlib builds, and mktime() would apply
// the local timezone to timestamps that are already UTC.
static long days_from_civil(int y, unsigned m, unsigned d) {
	y -= m <= 2;
	const long era = (y >= 0 ? y : y - 399) / 400;
	const unsigned yoe = (unsigned)(y - era * 400);
	const unsigned doy = (153u * (m + (m > 2 ? -3 : 9)) + 2) / 5 + d - 1;
	const unsigned doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
	return era * 146097L + (long)doe - 719468L;
}

// Parses "2026-08-05T17:54:25Z" and its fractional-second variants. Only UTC
// is accepted: every timestamp dasher publishes is UTC, and silently mangling
// an offset would be worse than showing nothing.
static time_t parse_rfc3339(const char *s) {
	if (s == nullptr) {
		return 0;
	}
	int year, mon, day, hour, min, sec;
	if (sscanf(s, "%4d-%2d-%2dT%2d:%2d:%2d", &year, &mon, &day, &hour, &min, &sec) != 6) {
		return 0;
	}
	if (strchr(s, 'Z') == nullptr) {
		return 0;
	}
	return (time_t)(days_from_civil(year, mon, day) * 86400L + hour * 3600L + min * 60L + sec);
}

bool dashboard_parse(const char *json, size_t len, Dashboard &out) {
	// The notification array can carry up to 50 entries. Every repo is read to
	// count distinct repositories, but only the newest few are kept in full
	// for the detail view. Filtering drops the timestamps and subject bodies
	// nothing renders.
	JsonDocument filter;
	filter["updated_at"] = true;
	filter["github"]["unread_notifications"] = true;
	filter["github"]["notifications"][0]["repo"] = true;
	filter["github"]["notifications"][0]["reason"] = true;
	filter["github"]["notifications"][0]["type"] = true;
	filter["github"]["notifications"][0]["updated_at"] = true;
	filter["claude"] = true;
	filter["weather"][0]["time"] = true;
	filter["weather"][0]["temperature_c"] = true;
	filter["weather"][0]["precipitation_mm"] = true;
	filter["weather"][0]["symbol"] = true;

	JsonDocument doc;
	DeserializationError err = deserializeJson(doc, json, len, DeserializationOption::Filter(filter));
	if (err) {
		Serial.printf("[payload] deserialize: %s\n", err.c_str());
		return false;
	}

	Dashboard d;
	d.updated_at = parse_rfc3339(doc["updated_at"] | (const char *)nullptr);
	d.received_ms = millis();

	JsonObjectConst gh = doc["github"].as<JsonObjectConst>();
	if (!gh.isNull()) {
		d.have_github = true;
		d.unread = gh["unread_notifications"] | 0;
		// Distinct repositories, counted by comparing each entry against the
		// ones before it. The list is short enough that O(n^2) is cheaper
		// than allocating a set.
		JsonArrayConst ns = gh["notifications"].as<JsonArrayConst>();
		const char *seen[24];
		int nseen = 0;
		for (JsonObjectConst n : ns) {
			const char *repo = n["repo"] | "";
			if (repo[0] == '\0') {
				continue;
			}
			// The API returns them newest first, so the first few are the
			// ones worth keeping.
			if (d.notification_count < kMaxNotifications) {
				Notification &e = d.notifications[d.notification_count++];
				snprintf(e.repo, sizeof(e.repo), "%s", repo);
				snprintf(e.reason, sizeof(e.reason), "%s", n["reason"] | "");
				snprintf(e.type, sizeof(e.type), "%s", n["type"] | "");
				e.updated_at = parse_rfc3339(n["updated_at"] | (const char *)nullptr);
			}
			bool dup = false;
			for (int i = 0; i < nseen; i++) {
				if (strcmp(seen[i], repo) == 0) {
					dup = true;
					break;
				}
			}
			if (!dup && nseen < (int)(sizeof(seen) / sizeof(seen[0]))) {
				seen[nseen++] = repo;
			}
		}
		d.repos = nseen;
	}

	JsonObjectConst c = doc["claude"].as<JsonObjectConst>();
	if (!c.isNull()) {
		d.have_claude = true;
		d.five_hour_pct = c["five_hour_pct"] | 0.0f;
		d.seven_day_pct = c["seven_day_pct"] | 0.0f;
		d.five_hour_resets_at = parse_rfc3339(c["five_hour_resets_at"] | (const char *)nullptr);
		d.seven_day_resets_at = parse_rfc3339(c["seven_day_resets_at"] | (const char *)nullptr);
	}

	JsonArrayConst w = doc["weather"].as<JsonArrayConst>();
	if (!w.isNull()) {
		for (JsonObjectConst h : w) {
			if (d.hour_count >= kMaxHours) {
				break;
			}
			HourForecast &f = d.hours[d.hour_count];
			f.at = parse_rfc3339(h["time"] | (const char *)nullptr);
			f.temperature_c = h["temperature_c"] | 0.0f;
			f.precipitation_mm = h["precipitation_mm"] | 0.0f;
			snprintf(f.symbol, sizeof(f.symbol), "%s", h["symbol"] | "");
			d.hour_count++;
		}
		d.have_weather = d.hour_count > 0;
	}

	out = d;
	return true;
}
