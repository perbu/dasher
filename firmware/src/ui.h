// Renders the dashboard. Everything is composed off-screen and pushed in one
// go, so the panel never shows a half-drawn frame.
#pragma once

#ifndef LGFX_USE_V1
#define LGFX_USE_V1
#endif
#include <LovyanGFX.hpp>

#include "dashboard.h"
#include "net.h"

// Boot/diagnostic screen, shown until the first payload arrives.
void ui_status(lgfx::LGFXBase &g, const char *headline, const char *detail);

// The dashboard itself, minus the footer band.
void ui_render(lgfx::LGFXBase &g, const Dashboard &d);

// The full-screen detail view: everything in the payload the dashboard has to
// summarise. Covers the footer band too, so it is drawn on its own.
void ui_render_detail(lgfx::LGFXBase &g, const Dashboard &d);

// The footer band, refreshed once a second without redrawing the frame. y0 is
// the band's top edge in g's coordinates: 0 when drawing into a band-sized
// sprite, kFooterY when drawing straight onto the panel.
void ui_render_footer(lgfx::LGFXBase &g, const Dashboard &d, const NetStatus &net, int y0);
