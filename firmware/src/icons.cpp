#include "icons.h"

#include <math.h>
#include <string.h>

#include "theme.h"

static void sun(lgfx::LGFXBase &g, int cx, int cy, int r, uint16_t c) {
	g.fillCircle(cx, cy, (int)(r * 0.52f), c);
	for (int i = 0; i < 8; i++) {
		float a = i * (float)M_PI / 4.0f;
		int x0 = cx + (int)(cosf(a) * r * 0.74f), y0 = cy + (int)(sinf(a) * r * 0.74f);
		int x1 = cx + (int)(cosf(a) * r * 1.02f), y1 = cy + (int)(sinf(a) * r * 1.02f);
		g.drawLine(x0, y0, x1, y1, c);
	}
}

// Crescent: a disc with a second disc punched out of it in the background
// colour. rad is the disc's radius, so callers control the size directly.
// Only correct over a flat background, which is what the cards are.
static void moon(lgfx::LGFXBase &g, int cx, int cy, int rad, uint16_t c, uint16_t bg) {
	g.fillCircle(cx, cy, rad, c);
	g.fillCircle(cx + (int)(rad * 0.58f), cy - (int)(rad * 0.42f), (int)(rad * 0.92f), bg);
}

// pad dilates the shape. Drawing it once in the background colour with pad>0
// and again in the real colour with pad=0 leaves a dark gap around the cloud,
// which is what makes it legible where it overlaps a sun or moon. The panel's
// contrast is poor enough that two similar greys just merge.
static void cloud_shape(lgfx::LGFXBase &g, int cx, int cy, int r, uint16_t c, int pad) {
	g.fillCircle(cx - (int)(r * 0.46f), cy, (int)(r * 0.44f) + pad, c);
	g.fillCircle(cx + (int)(r * 0.42f), cy, (int)(r * 0.38f) + pad, c);
	g.fillCircle(cx - (int)(r * 0.02f), cy - (int)(r * 0.30f), (int)(r * 0.54f) + pad, c);
	g.fillRect(cx - (int)(r * 0.46f) - pad, cy - (int)(r * 0.04f) - pad,
	           (int)(r * 0.90f) + 2 * pad, (int)(r * 0.46f) + pad, c);
}

static void cloud(lgfx::LGFXBase &g, int cx, int cy, int r, uint16_t c) {
	cloud_shape(g, cx, cy, r, c, 0);
}

static void rain(lgfx::LGFXBase &g, int cx, int cy, int r, int drops, uint16_t c) {
	int top = cy + (int)(r * 0.48f);
	int len = (int)(r * 0.42f);
	for (int i = 0; i < drops; i++) {
		int x = cx + (int)((i - (drops - 1) / 2.0f) * r * 0.44f);
		g.drawLine(x + 2, top, x - 1, top + len, c);
	}
}

static void snow(lgfx::LGFXBase &g, int cx, int cy, int r, uint16_t c) {
	int y = cy + (int)(r * 0.66f);
	for (int i = -1; i <= 1; i++) {
		int x = cx + (int)(i * r * 0.46f);
		g.drawLine(x - 2, y, x + 2, y, c);
		g.drawLine(x, y - 2, x, y + 2, c);
	}
}

static void bolt(lgfx::LGFXBase &g, int cx, int cy, int r, uint16_t c) {
	int top = cy + (int)(r * 0.42f);
	g.fillTriangle(cx + 2, top, cx - 4, top + (int)(r * 0.46f), cx + 1, top + (int)(r * 0.44f), c);
	g.fillTriangle(cx + 1, top + (int)(r * 0.30f), cx + 5, top + (int)(r * 0.30f), cx - 2, top + (int)(r * 0.86f), c);
}

static void fog(lgfx::LGFXBase &g, int cx, int cy, int r, uint16_t c) {
	for (int i = 0; i < 3; i++) {
		int y = cy + (int)(r * 0.46f) + i * 5;
		int w = (int)(r * (i == 1 ? 0.70f : 0.88f));
		g.drawFastHLine(cx - w / 2, y, w, c);
	}
}

void icon_draw(lgfx::LGFXBase &g, const char *symbol_code, int cx, int cy, int r) {
	if (symbol_code == nullptr || symbol_code[0] == '\0') {
		g.drawCircle(cx, cy, (int)(r * 0.7f), kDim);
		return;
	}
	const bool night = strstr(symbol_code, "_night") != nullptr;
	const bool heavy = strncmp(symbol_code, "heavy", 5) == 0;
	const uint16_t disc = night ? kMoon : kSun;
	// Nudged up so the precipitation strokes below stay inside the cell.
	const int sky_y = cy - (int)(r * 0.22f);

	// Order matters: met.no composes codes like "lightsnowshowersandthunder_day",
	// so the most specific term has to win.
	if (strstr(symbol_code, "thunder")) {
		cloud(g, cx, sky_y, r, kCloud);
		bolt(g, cx, sky_y, r, kSun);
		return;
	}
	if (strstr(symbol_code, "sleet")) {
		cloud(g, cx, sky_y, r, kCloud);
		rain(g, cx, sky_y, r, 2, kBlue);
		snow(g, cx + (int)(r * 0.30f), sky_y, r, kSnow);
		return;
	}
	if (strstr(symbol_code, "snow")) {
		cloud(g, cx, sky_y, r, kCloud);
		snow(g, cx, sky_y, r, kSnow);
		return;
	}
	if (strstr(symbol_code, "rain")) {
		cloud(g, cx, sky_y, r, kCloud);
		rain(g, cx, sky_y, r, heavy ? 4 : 3, kBlue);
		return;
	}
	if (strstr(symbol_code, "fog")) {
		cloud(g, cx, sky_y, r, kCloud);
		fog(g, cx, sky_y, r, kCloud);
		return;
	}
	if (strstr(symbol_code, "partlycloudy") || strstr(symbol_code, "fair")) {
		// Sun or moon peeking out behind the cloud, both scaled down so the
		// composite does not turn into one blob at this size.
		if (night) {
			moon(g, cx + (int)(r * 0.44f), cy - (int)(r * 0.44f), (int)(r * 0.46f), disc, kCard);
		} else {
			sun(g, cx + (int)(r * 0.44f), cy - (int)(r * 0.44f), (int)(r * 0.58f), disc);
		}
		// Dark gap first, then the cloud itself.
		cloud_shape(g, cx - (int)(r * 0.16f), cy + (int)(r * 0.16f), (int)(r * 0.86f), kCard, 2);
		cloud_shape(g, cx - (int)(r * 0.16f), cy + (int)(r * 0.16f), (int)(r * 0.86f), kCloud, 0);
		return;
	}
	if (strstr(symbol_code, "cloudy")) {
		cloud(g, cx, cy, r, kCloud);
		return;
	}
	if (strstr(symbol_code, "clearsky")) {
		if (night) {
			moon(g, cx, cy, (int)(r * 0.78f), disc, kCard);
		} else {
			sun(g, cx, cy, r, disc);
		}
		return;
	}
	// Unknown code: a hollow disc reads as "no idea" without looking broken.
	g.drawCircle(cx, cy, (int)(r * 0.7f), kDim);
}
