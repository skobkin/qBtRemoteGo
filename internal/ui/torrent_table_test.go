package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
)

func TestRowTapSequencer(t *testing.T) {
	start := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	t.Run("single tap does not trigger", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		if s.record("a", start, 0) {
			t.Fatal("first tap must not complete a double-click")
		}
	})

	t.Run("same hash within interval triggers", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start, 0)
		if !s.record("a", start.Add(100*time.Millisecond), 0) {
			t.Fatal("second tap on the same row within the interval must trigger")
		}
	})

	t.Run("different hash re-anchors instead of triggering", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start, 0)
		if s.record("b", start.Add(100*time.Millisecond), 0) {
			t.Fatal("a tap on a different row must not complete the previous row's double-click")
		}
		if !s.record("b", start.Add(200*time.Millisecond), 0) {
			t.Fatal("second tap on the new row must trigger")
		}
	})

	t.Run("tap beyond the interval re-anchors", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start, 0)
		if s.record("a", start.Add(torrentDoubleTapInterval+time.Millisecond), 0) {
			t.Fatal("a tap after the interval must not trigger")
		}
		if !s.record("a", start.Add(2*torrentDoubleTapInterval+time.Millisecond), 0) {
			t.Fatal("next tap within the interval must trigger")
		}
	})

	t.Run("triple tap opens details only once", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start, 0)
		if !s.record("a", start.Add(100*time.Millisecond), 0) {
			t.Fatal("second tap must trigger")
		}
		if s.record("a", start.Add(200*time.Millisecond), 0) {
			t.Fatal("third tap must start a new sequence, not trigger again")
		}
	})

	t.Run("modified tap never triggers", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start, 0)
		if s.record("a", start.Add(100*time.Millisecond), fyne.KeyModifierControl) {
			t.Fatal("a ctrl tap on the same row must not complete a double-click; it toggles the selection")
		}
	})

	t.Run("modified tap breaks a pending sequence", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start, 0)
		s.record("a", start.Add(100*time.Millisecond), fyne.KeyModifierControl)
		if s.record("a", start.Add(200*time.Millisecond), 0) {
			t.Fatal("a plain tap after a modifier tap must re-anchor, not fuse with the earlier plain tap")
		}
		if !s.record("a", start.Add(300*time.Millisecond), 0) {
			t.Fatal("second plain tap after the re-anchor must trigger")
		}
	})

	t.Run("shift tap never triggers", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start, 0)
		if s.record("a", start.Add(100*time.Millisecond), fyne.KeyModifierShift) {
			t.Fatal("a shift tap must not complete a double-click; it extends the selection")
		}
	})
}
