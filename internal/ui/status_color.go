package ui

import (
	"image/color"
	"math"
)

// statusChipDarkText is the dark chip text colour, used on light chips such as
// the amber queued/checking one where white text would be unreadable.
var statusChipDarkText = color.NRGBA{R: 0x1a, G: 0x1a, B: 0x1a, A: 0xff}

// statusTextColor picks white or near-black chip text for a status colour,
// whichever contrasts more. Chip text is drawn in a fixed colour instead of
// the theme foreground so a light theme cannot put dark text on a dark chip.
func statusTextColor(bg color.Color) color.Color {
	l := luminance(bg)
	whiteContrast := 1.05 / (l + 0.05)
	darkContrast := (l + 0.05) / (luminance(statusChipDarkText) + 0.05)
	if darkContrast > whiteContrast {
		return statusChipDarkText
	}
	return color.White
}

// luminance is the WCAG relative luminance of c.
func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	channel := func(v uint32) float64 {
		s := float64(v) / 0xffff
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(r) + 0.7152*channel(g) + 0.0722*channel(b)
}
