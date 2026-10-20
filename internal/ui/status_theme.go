package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// compactStatusTheme inner padding used for the status bar row.
const compactStatusInnerPadding = float32(2)

// compactStatusTheme resolves sizes through the captured app theme, but with
// a tighter inner padding so the status bar row hugs its text. Text size,
// colors, and fonts are unchanged.
//
// The base theme is captured at construction time: resolving it lazily via
// theme.Current() would recurse, because the override itself is the current
// rendering theme while its widgets draw.
type compactStatusTheme struct {
	fyne.Theme
}

func newCompactStatusTheme() compactStatusTheme {
	base := fyne.CurrentApp().Settings().Theme()
	if base == nil {
		base = theme.DefaultTheme()
	}
	return compactStatusTheme{Theme: base}
}

func (t compactStatusTheme) Size(name fyne.ThemeSizeName) float32 {
	if name == theme.SizeNameInnerPadding {
		return compactStatusInnerPadding
	}
	return t.Theme.Size(name)
}
