#include "ui.h"

#include <stdio.h>
#include <string.h>
#include <time.h>

#include "icons.h"
#include "theme.h"

// Utilisation shades from calm to alarming. The thresholds are about glancing
// value, not policy: below half is fine, above 85% is worth reacting to.
static uint16_t usage_color(float pct) {
	if (pct >= 85.0f) {
		return kRed;
	}
	if (pct >= 50.0f) {
		return kAmber;
	}
	return kGreen;
}

static void card(lgfx::LGFXBase &g, int x, int y, int w, int h) {
	g.fillRoundRect(x, y, w, h, 8, kCard);
	g.drawRoundRect(x, y, w, h, 8, kStroke);
}

static void label(lgfx::LGFXBase &g, const char *s, int x, int y, uint16_t bg) {
	g.setFont(&fonts::DejaVu12);
	g.setTextColor(kDim, bg);
	g.setTextDatum(textdatum_t::middle_left);
	g.drawString(s, x, y);
}

// "22:30" for a reset later today, "Sun 15:00" once it crosses midnight.
// Local time: the payload is UTC and TZ_SPEC is applied via setenv in main.
static void format_reset(time_t t, time_t now, char *buf, size_t n) {
	if (t == 0) {
		snprintf(buf, n, "--:--");
		return;
	}
	struct tm lt, ln;
	localtime_r(&t, &lt);
	localtime_r(&now, &ln);
	if (lt.tm_yday == ln.tm_yday && lt.tm_year == ln.tm_year) {
		strftime(buf, n, "%H:%M", &lt);
	} else {
		strftime(buf, n, "%a %H:%M", &lt);
	}
}

// A ring gauge: full circle, filled clockwise from 12 o'clock. LovyanGFX
// measures degrees clockwise from 3 o'clock, hence the -90 offset.
static void gauge(lgfx::LGFXBase &g, int cx, int cy, float pct, bool have, const char *name, const char *reset) {
	const int r_out = 42, r_in = 31;
	g.fillArc(cx, cy, r_in, r_out, 0, 360, kTrack);
	if (have) {
		float clamped = pct < 0 ? 0 : (pct > 100 ? 100 : pct);
		if (clamped > 0.5f) {
			g.fillArc(cx, cy, r_in, r_out, -90, -90 + 3.6f * clamped, usage_color(clamped));
		}
	}

	char buf[8];
	if (have) {
		snprintf(buf, sizeof(buf), "%d%%", (int)(pct + 0.5f));
	} else {
		snprintf(buf, sizeof(buf), "--");
	}
	g.setFont(&fonts::DejaVu24);
	g.setTextColor(have ? kText : kDim, kCard);
	g.setTextDatum(textdatum_t::middle_center);
	g.drawString(buf, cx, cy);

	g.setFont(&fonts::DejaVu12);
	g.setTextColor(kDim, kCard);
	char line[32];
	snprintf(line, sizeof(line), "%s  %s", name, reset);
	g.drawString(line, cx, cy + 58);
}

void ui_status(lgfx::LGFXBase &g, const char *headline, const char *detail) {
	g.fillScreen(kBg);
	g.setTextDatum(textdatum_t::middle_center);
	g.setFont(&fonts::DejaVu24);
	g.setTextColor(kText, kBg);
	g.drawString("dasher", kScreenW / 2, 116);
	g.setFont(&fonts::DejaVu18);
	g.setTextColor(kBlue, kBg);
	g.drawString(headline, kScreenW / 2, 164);
	if (detail != nullptr && detail[0] != '\0') {
		g.setFont(&fonts::DejaVu12);
		g.setTextColor(kDim, kBg);
		g.drawString(detail, kScreenW / 2, 194);
	}
}

static void render_claude(lgfx::LGFXBase &g, const Dashboard &d, time_t now) {
	card(g, kClaudeX, kClaudeY, kClaudeW, kClaudeH);
	label(g, "CLAUDE", kClaudeX + 16, kClaudeY + 18, kCard);

	char five[16], seven[16];
	format_reset(d.five_hour_resets_at, now, five, sizeof(five));
	format_reset(d.seven_day_resets_at, now, seven, sizeof(seven));

	const int cy = kClaudeY + 88;
	gauge(g, kClaudeX + 78, cy, d.five_hour_pct, d.have_claude, "5h", five);
	gauge(g, kClaudeX + 206, cy, d.seven_day_pct, d.have_claude, "7d", seven);
}

static void render_github(lgfx::LGFXBase &g, const Dashboard &d) {
	card(g, kGitHubX, kGitHubY, kGitHubW, kGitHubH);
	label(g, "GITHUB", kGitHubX + 16, kGitHubY + 18, kCard);

	const int cx = kGitHubX + kGitHubW / 2;
	char buf[16];
	if (d.have_github) {
		// The GitHub API caps the page at 50, so the count saturates there.
		snprintf(buf, sizeof(buf), d.unread >= 50 ? "50+" : "%d", d.unread);
	} else {
		snprintf(buf, sizeof(buf), "--");
	}
	g.setFont(&fonts::FreeSansBold24pt7b);
	g.setTextColor(d.have_github && d.unread > 0 ? kText : kDim, kCard);
	g.setTextDatum(textdatum_t::middle_center);
	g.drawString(buf, cx, kGitHubY + 78);

	g.setFont(&fonts::DejaVu12);
	g.setTextColor(kDim, kCard);
	g.drawString("unread", cx, kGitHubY + 116);
	if (d.have_github && d.repos > 0) {
		snprintf(buf, sizeof(buf), "%d %s", d.repos, d.repos == 1 ? "repo" : "repos");
		g.drawString(buf, cx, kGitHubY + 140);
	}
}

static void render_weather(lgfx::LGFXBase &g, const Dashboard &d) {
	card(g, kWeatherX, kWeatherY, kWeatherW, kWeatherH);
	if (!d.have_weather) {
		g.setFont(&fonts::DejaVu12);
		g.setTextColor(kDim, kCard);
		g.setTextDatum(textdatum_t::middle_center);
		g.drawString("no forecast yet", kWeatherX + kWeatherW / 2, kWeatherY + kWeatherH / 2);
		return;
	}

	const int cols = d.hour_count;
	const int col_w = kWeatherW / (cols > 0 ? cols : 1);
	for (int i = 0; i < cols; i++) {
		const HourForecast &h = d.hours[i];
		const int cx = kWeatherX + col_w * i + col_w / 2;
		if (i > 0) {
			g.drawFastVLine(kWeatherX + col_w * i, kWeatherY + 12, kWeatherH - 24, kStroke);
		}

		char buf[16];
		struct tm lt;
		localtime_r(&h.at, &lt);
		strftime(buf, sizeof(buf), "%H", &lt);
		g.setFont(&fonts::DejaVu12);
		g.setTextColor(kDim, kCard);
		g.setTextDatum(textdatum_t::middle_center);
		g.drawString(buf, cx, kWeatherY + 13);

		// As large as the cell allows: small glyphs are the first thing to
		// become unreadable on this panel.
		icon_draw(g, h.symbol, cx, kWeatherY + 42, 16);

		snprintf(buf, sizeof(buf), "%.1f", h.temperature_c);
		g.setFont(&fonts::DejaVu18);
		g.setTextColor(h.temperature_c < 0 ? kBlue : kText, kCard);
		g.drawString(buf, cx, kWeatherY + 69);

		// Dry hours get a dot rather than "0.0mm", so the wet ones stand out.
		g.setFont(&fonts::DejaVu12);
		if (h.precipitation_mm > 0.05f) {
			snprintf(buf, sizeof(buf), "%.1f mm", h.precipitation_mm);
			g.setTextColor(kBlue, kCard);
		} else {
			snprintf(buf, sizeof(buf), "-");
			g.setTextColor(kStroke, kCard);
		}
		g.drawString(buf, cx, kWeatherY + 87);
	}
}

void ui_render(lgfx::LGFXBase &g, const Dashboard &d) {
	const time_t now = time(nullptr);
	g.fillScreen(kBg);
	render_claude(g, d, now);
	render_github(g, d);
	render_weather(g, d);
}

// "Fri 8 Aug 22:30" — the dashboard gauges only have room for the time, and
// the whole point of the detail view is not having to guess the day.
static void format_reset_full(time_t t, char *buf, size_t n) {
	if (t == 0) {
		snprintf(buf, n, "unknown");
		return;
	}
	struct tm lt;
	localtime_r(&t, &lt);
	strftime(buf, n, "%a %e %b %H:%M", &lt);
}

// met.no codes carry a "_day"/"_night" suffix that says nothing the icon has
// not already said. Drop it so the name fits the column.
static void symbol_label(const char *code, char *buf, size_t n) {
	snprintf(buf, n, "%s", code == nullptr ? "" : code);
	char *u = strrchr(buf, '_');
	if (u != nullptr && (strcmp(u, "_day") == 0 || strcmp(u, "_night") == 0 || strcmp(u, "_polartwilight") == 0)) {
		*u = '\0';
	}
}

void ui_render_detail(lgfx::LGFXBase &g, const Dashboard &d) {
	g.fillScreen(kBg);

	g.setFont(&fonts::DejaVu18);
	g.setTextColor(kText, kBg);
	g.setTextDatum(textdatum_t::middle_left);
	g.drawString("details", 16, 15);
	g.setFont(&fonts::DejaVu12);
	g.setTextColor(kDim, kBg);
	g.setTextDatum(textdatum_t::middle_right);
	g.drawString("tap to close", kScreenW - 16, 15);
	g.drawFastHLine(8, 29, kScreenW - 16, kStroke);

	char buf[64];

	// Left: the notification list, which the dashboard can only count.
	const int lx = 8, ly = 36, lw = 238, lh = 276;
	card(g, lx, ly, lw, lh);
	label(g, "GITHUB", lx + 16, ly + 18, kCard);
	g.setTextDatum(textdatum_t::middle_left);
	if (!d.have_github) {
		g.setFont(&fonts::DejaVu12);
		g.setTextColor(kDim, kCard);
		g.drawString("no data yet", lx + 16, ly + 44);
	} else if (d.unread == 0) {
		g.setFont(&fonts::DejaVu12);
		g.setTextColor(kGreen, kCard);
		g.drawString("inbox clear", lx + 16, ly + 44);
	} else {
		snprintf(buf, sizeof(buf), "%d unread in %d %s", d.unread, d.repos, d.repos == 1 ? "repo" : "repos");
		g.setFont(&fonts::DejaVu12);
		g.setTextColor(kDim, kCard);
		g.drawString(buf, lx + 16, ly + 42);

		int y = ly + 70;
		for (uint8_t i = 0; i < d.notification_count; i++) {
			const Notification &n = d.notifications[i];
			g.setFont(&fonts::DejaVu12);
			g.setTextColor(kText, kCard);
			g.drawString(n.repo, lx + 16, y);
			snprintf(buf, sizeof(buf), "%s%s%s", n.reason, n.type[0] ? " \xC2\xB7 " : "", n.type);
			g.setFont(&fonts::DejaVu9);
			g.setTextColor(kDim, kCard);
			g.drawString(buf, lx + 16, y + 15);
			y += 34;
		}
		if (d.unread > d.notification_count) {
			snprintf(buf, sizeof(buf), "+%d more", d.unread - d.notification_count);
			g.setFont(&fonts::DejaVu9);
			g.setTextColor(kDim, kCard);
			g.drawString(buf, lx + 16, y);
		}
	}

	// Top right: Claude, with the dates the gauges have no room for.
	const int rx = 254, rw = 218;
	card(g, rx, 36, rw, 104);
	label(g, "CLAUDE", rx + 16, 54, kCard);
	g.setTextDatum(textdatum_t::middle_left);
	if (!d.have_claude) {
		g.setFont(&fonts::DejaVu12);
		g.setTextColor(kDim, kCard);
		g.drawString("no data yet", rx + 16, 82);
	} else {
		const struct {
			const char *name;
			float pct;
			time_t reset;
		} windows[2] = {
		    {"5-hour", d.five_hour_pct, d.five_hour_resets_at},
		    {"7-day", d.seven_day_pct, d.seven_day_resets_at},
		};
		int y = 76;
		for (int i = 0; i < 2; i++) {
			// One decimal: the gauge rounds, and the difference matters when
			// you are deciding whether to start something.
			snprintf(buf, sizeof(buf), "%s  %.1f%%", windows[i].name, windows[i].pct);
			g.setFont(&fonts::DejaVu12);
			g.setTextColor(usage_color(windows[i].pct), kCard);
			g.drawString(buf, rx + 16, y);
			format_reset_full(windows[i].reset, buf, sizeof(buf));
			g.setFont(&fonts::DejaVu9);
			g.setTextColor(kDim, kCard);
			g.drawString(buf, rx + 16, y + 15);
			y += 36;
		}
	}

	// Bottom right: the forecast spelled out, so an ambiguous icon can always
	// be resolved by reading the name.
	card(g, rx, 148, rw, 164);
	label(g, "WEATHER", rx + 16, 166, kCard);
	g.setTextDatum(textdatum_t::middle_left);
	if (!d.have_weather) {
		g.setFont(&fonts::DejaVu12);
		g.setTextColor(kDim, kCard);
		g.drawString("no forecast yet", rx + 16, 194);
	} else {
		int y = 190;
		for (uint8_t i = 0; i < d.hour_count; i++) {
			const HourForecast &h = d.hours[i];
			struct tm lt;
			localtime_r(&h.at, &lt);
			strftime(buf, sizeof(buf), "%H", &lt);
			g.setFont(&fonts::DejaVu12);
			g.setTextColor(kDim, kCard);
			g.drawString(buf, rx + 16, y);

			snprintf(buf, sizeof(buf), "%.1f\xC2\xB0", h.temperature_c);
			g.setTextColor(h.temperature_c < 0 ? kBlue : kText, kCard);
			g.drawString(buf, rx + 44, y);

			symbol_label(h.symbol, buf, sizeof(buf));
			g.setFont(&fonts::DejaVu9);
			g.setTextColor(kDim, kCard);
			g.drawString(buf, rx + 96, y);

			if (h.precipitation_mm > 0.05f) {
				snprintf(buf, sizeof(buf), "%.1f", h.precipitation_mm);
				g.setTextColor(kBlue, kCard);
				g.setTextDatum(textdatum_t::middle_right);
				g.drawString(buf, rx + rw - 12, y);
				g.setTextDatum(textdatum_t::middle_left);
			}
			y += 20;
		}
	}
}

void ui_render_footer(lgfx::LGFXBase &g, const Dashboard &d, const NetStatus &net, int y0) {
	const int mid = y0 + kFooterH / 2;
	g.fillRect(0, y0, kScreenW, kFooterH, kBg);
	g.setFont(&fonts::DejaVu12);
	g.setTextDatum(textdatum_t::middle_left);

	char buf[48];
	uint16_t age_color = kDim;
	if (d.updated_at == 0) {
		snprintf(buf, sizeof(buf), "waiting for data");
		age_color = kAmber;
	} else {
		// Age from the payload's own timestamp, so a stalled fetcher on the
		// server shows up here even while MQTT stays happily connected.
		long age = (long)(time(nullptr) - d.updated_at);
		if (age < 0) {
			age = 0;
		}
		if (age < 90) {
			snprintf(buf, sizeof(buf), "updated %lds ago", age);
		} else if (age < 3600) {
			snprintf(buf, sizeof(buf), "updated %ldm ago", age / 60);
			age_color = kAmber;
		} else {
			snprintf(buf, sizeof(buf), "updated %ldh ago", age / 3600);
			age_color = kRed;
		}
	}
	g.setTextColor(age_color, kBg);
	g.drawString(buf, kClaudeX + 8, mid);

	const char *state;
	uint16_t state_color;
	if (!net.wifi_up) {
		state = "wifi down";
		state_color = kRed;
	} else if (!net.time_ok) {
		state = "syncing clock";
		state_color = kAmber;
	} else if (!net.mqtt_up) {
		state = net.detail[0] ? net.detail : "mqtt down";
		state_color = kRed;
	} else {
		state = "mqtt ok";
		state_color = kGreen;
	}
	if (net.wifi_up) {
		snprintf(buf, sizeof(buf), "%s  %d dBm", state, net.rssi);
	} else {
		snprintf(buf, sizeof(buf), "%s", state);
	}
	g.setTextDatum(textdatum_t::middle_right);
	g.setTextColor(state_color, kBg);
	g.drawString(buf, kScreenW - kClaudeX - 8, mid);
}
