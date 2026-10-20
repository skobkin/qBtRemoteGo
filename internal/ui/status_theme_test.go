package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func TestCompactStatusThemeOnlyTightensInnerPadding(t *testing.T) {
	test.NewApp()
	th := newCompactStatusTheme()

	for _, name := range []fyne.ThemeSizeName{
		theme.SizeNameText,
		theme.SizeNamePadding,
		theme.SizeNameLineSpacing,
		theme.SizeNameInlineIcon,
	} {
		if got, want := th.Size(name), theme.Current().Size(name); got != want {
			t.Errorf("Size(%v) = %v, want %v", name, got, want)
		}
	}

	if got, want := th.Size(theme.SizeNameInnerPadding), compactStatusInnerPadding; got != want {
		t.Errorf("Size(InnerPadding) = %v, want %v", got, want)
	}
}

func TestCompactStatusThemeShrinksLabel(t *testing.T) {
	test.NewApp()

	label := widget.NewLabel("Connected · ↓ 1.2 MiB/s · Free 1.5 TiB")
	plainHeight := label.MinSize().Height

	compact := container.NewThemeOverride(label, newCompactStatusTheme())
	compactHeight := compact.MinSize().Height

	want := plainHeight - (theme.Current().Size(theme.SizeNameInnerPadding)-compactStatusInnerPadding)*2
	if compactHeight != want {
		t.Errorf("compact label height = %v, want %v", compactHeight, want)
	}
	if compactHeight >= plainHeight {
		t.Errorf("compact label height %v should be smaller than plain %v", compactHeight, plainHeight)
	}
}
