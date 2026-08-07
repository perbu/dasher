// Weather glyphs, drawn as vectors rather than embedded bitmaps so they scale
// with the layout and cost no flash.
#pragma once

#ifndef LGFX_USE_V1
#define LGFX_USE_V1
#endif
#include <LovyanGFX.hpp>

// Draws the icon for a met.no symbol_code (e.g. "lightrainshowers_day")
// centred on (cx, cy), fitting roughly a 2r box.
void icon_draw(lgfx::LGFXBase &g, const char *symbol_code, int cx, int cy, int r);
