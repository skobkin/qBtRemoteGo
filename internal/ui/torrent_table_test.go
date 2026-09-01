package ui

import (
	"testing"
	"time"
)

func TestRowTapSequencer(t *testing.T) {
	start := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	t.Run("single tap does not trigger", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		if s.record("a", start) {
			t.Fatal("first tap must not complete a double-click")
		}
	})

	t.Run("same hash within interval triggers", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start)
		if !s.record("a", start.Add(100*time.Millisecond)) {
			t.Fatal("second tap on the same row within the interval must trigger")
		}
	})

	t.Run("different hash re-anchors instead of triggering", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start)
		if s.record("b", start.Add(100*time.Millisecond)) {
			t.Fatal("a tap on a different row must not complete the previous row's double-click")
		}
		if !s.record("b", start.Add(200*time.Millisecond)) {
			t.Fatal("second tap on the new row must trigger")
		}
	})

	t.Run("tap beyond the interval re-anchors", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start)
		if s.record("a", start.Add(torrentDoubleTapInterval+time.Millisecond)) {
			t.Fatal("a tap after the interval must not trigger")
		}
		if !s.record("a", start.Add(2*torrentDoubleTapInterval+time.Millisecond)) {
			t.Fatal("next tap within the interval must trigger")
		}
	})

	t.Run("triple tap opens details only once", func(t *testing.T) {
		var s rowTapSequencer
		s.interval = torrentDoubleTapInterval
		s.record("a", start)
		if !s.record("a", start.Add(100*time.Millisecond)) {
			t.Fatal("second tap must trigger")
		}
		if s.record("a", start.Add(200*time.Millisecond)) {
			t.Fatal("third tap must start a new sequence, not trigger again")
		}
	})
}
