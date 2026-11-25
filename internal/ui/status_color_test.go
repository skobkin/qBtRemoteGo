package ui

import (
	"image/color"
	"math"
	"testing"

	"github.com/skobkin/qbtremotego/internal/qbt"
)

// wcagContrast returns the WCAG 2.x contrast ratio between two colours.
func wcagContrast(a, b color.Color) float64 {
	la, lb := testLuminance(a), testLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func testLuminance(c color.Color) float64 {
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

func TestStatusChipTextMeetsContrastMinimum(t *testing.T) {
	states := []string{
		"downloading", "metaDL", "stalledDL", "uploading", "stalledUP",
		"queuedDL", "checkingDL", "stoppedDL", "pausedUP", "moving",
		"missingFiles", "error", "unknown-state",
	}
	for _, state := range states {
		bg := statusColor(state)
		fg := statusTextColor(bg)
		if got := wcagContrast(bg, fg); got < 4.5 {
			t.Errorf("state %q: chip text contrast %.2f:1, want >= 4.5:1", state, got)
		}
	}
}

// The chip text colour follows the chip background, not the theme foreground.
func TestTorrentRowStatusChipUsesContrastingTextColor(t *testing.T) {
	app := newTorrentTableTestApp(t)
	row := app.rowForTest(t, "hash-a")

	row.setTorrent(qbt.Torrent{Hash: "hash-a", State: "queuedDL"})
	if row.statusTx.Color != statusChipDarkText {
		t.Fatalf("queued chip text colour %v, want dark text", row.statusTx.Color)
	}

	row.setTorrent(qbt.Torrent{Hash: "hash-a", State: "downloading"})
	if row.statusTx.Color != color.White {
		t.Fatalf("downloading chip text colour %v, want white", row.statusTx.Color)
	}
}
