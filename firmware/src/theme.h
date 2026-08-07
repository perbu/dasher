// Palette and layout geometry for the 480x320 dashboard.
#pragma once

#include <stdint.h>

#define RGB565(r, g, b) ((uint16_t)((((r) & 0xF8) << 8) | (((g) & 0xFC) << 3) | ((b) >> 3)))

// Tuned for the panel, not for a monitor: its contrast ratio is low enough
// that mid-tones collapse into the background. Everything meant to be read is
// pushed bright, and shapes are separated by gaps in the background colour
// rather than by neighbouring shades of grey.
static const uint16_t kBg = RGB565(0x08, 0x0b, 0x10);
static const uint16_t kCard = RGB565(0x16, 0x1b, 0x22);
static const uint16_t kStroke = RGB565(0x3d, 0x47, 0x54);
static const uint16_t kTrack = RGB565(0x3a, 0x43, 0x4f); // unfilled part of a gauge
static const uint16_t kText = RGB565(0xf4, 0xf8, 0xfc);
static const uint16_t kDim = RGB565(0xa6, 0xb2, 0xc0);
static const uint16_t kGreen = RGB565(0x56, 0xd3, 0x64);
static const uint16_t kAmber = RGB565(0xf0, 0xb4, 0x29);
static const uint16_t kRed = RGB565(0xff, 0x6b, 0x5e);
static const uint16_t kBlue = RGB565(0x6c, 0xb6, 0xff);
static const uint16_t kSun = RGB565(0xff, 0xc4, 0x3c);
static const uint16_t kMoon = RGB565(0xf2, 0xf6, 0xfa);
static const uint16_t kCloud = RGB565(0xae, 0xba, 0xc7);
static const uint16_t kSnow = RGB565(0xee, 0xf4, 0xfa);

static const int kScreenW = 480;
static const int kScreenH = 320;

// Claude card (two gauges), GitHub card, weather strip, then a footer band
// that is redrawn on its own once a second to age the "updated" line.
static const int kClaudeX = 8, kClaudeY = 8, kClaudeW = 284, kClaudeH = 168;
static const int kGitHubX = 300, kGitHubY = 8, kGitHubW = 172, kGitHubH = 168;
static const int kWeatherX = 8, kWeatherY = 184, kWeatherW = 464, kWeatherH = 96;
static const int kFooterY = 288, kFooterH = 28;
