package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// A wrapped label reports a one-character minimum width and, once laid out
// narrow, a minimum height of one line per character. Any label placed in a
// min-size layout (Center) therefore collapses to a vertical character column
// and, inside the bottom details pane, inflates the pane's minimum until the
// VSplit squeezes the torrent table to a sliver. These tests pin the minimum
// sizes of the two wrapped-label spots in the details panel.

func TestDetailsEmptyStateKeepsReadableMinSize(t *testing.T) {
	test.NewTempApp(t)
	app := &application{
		controller:   newPollTestController(t),
		detailsState: newTorrentDetailsState(),
	}

	empty := newTorrentDetailsView(app).emptyState()
	min := empty.MinSize()

	if min.Width < 200 {
		t.Fatalf("placeholder minimum width %.0f collapsed below a readable line", min.Width)
	}
	if min.Height > 100 {
		t.Fatalf("placeholder minimum height %.0f reads as one line per character", min.Height)
	}
}

func TestDetailsStatusChromeWrapsInsteadOfCollapsing(t *testing.T) {
	test.NewTempApp(t)
	app := &application{detailsState: newTorrentDetailsState()}

	chrome := newDetailsStatusChrome(app)
	chrome.label.SetText("Failed to load details: " + strings.Repeat("the tracker sent malformed data ", 8))

	// A narrow layout pass like the overlay drawer, then re-measure: the label
	// must have taken the panel's width and the chrome's minimum must stay
	// within one panel height.
	chrome.root.Resize(fyne.NewSize(240, 400))

	if got := chrome.label.Size().Width; got < 200 {
		t.Fatalf("status label width %.0f collapsed instead of filling the panel", got)
	}
	if got := chrome.root.MinSize().Height; got > 400 {
		t.Fatalf("status chrome minimum height %.0f inflated beyond the panel", got)
	}
}
