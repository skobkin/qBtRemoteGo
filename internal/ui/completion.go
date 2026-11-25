package ui

import (
	"fmt"
	"log/slog"
	"strings"

	appcore "github.com/skobkin/qbtremotego/internal/app"
	"github.com/skobkin/qbtremotego/internal/notify"
	"github.com/skobkin/qbtremotego/internal/qbt"
)

// finishedNotificationMaxNames caps how many torrent names one notification
// lists.
const finishedNotificationMaxNames = 3

// completionTracker remembers each torrent's progress between polls so a
// torrent that crosses 100% can be reported once. The first poll after start
// or a reconnect only records a baseline, and torrents that show up already
// complete are never reported.
type completionTracker struct {
	progress map[string]float64
	primed   bool
}

// observe records a poll and returns the torrents that finished since the
// previous one.
func (t *completionTracker) observe(torrents []qbt.Torrent) []qbt.Torrent {
	next := make(map[string]float64, len(torrents))
	var finished []qbt.Torrent
	for _, torrent := range torrents {
		next[torrent.Hash] = torrent.Progress
		if !t.primed {
			continue
		}
		previous, known := t.progress[torrent.Hash]
		if known && previous < 1 && torrent.Progress >= 1 {
			finished = append(finished, torrent)
		}
	}
	t.progress = next
	t.primed = true
	return finished
}

// reset forgets the baseline, e.g. after a failed poll, so stale progress
// can't produce a burst of notifications on reconnect.
func (t *completionTracker) reset() {
	t.progress = nil
	t.primed = false
}

// finishedNotificationText builds one notification for a batch of finished
// torrents.
func finishedNotificationText(finished []qbt.Torrent) (string, string) {
	if len(finished) == 1 {
		return "Download finished", finished[0].Name
	}
	lines := make([]string, 0, finishedNotificationMaxNames+1)
	for i, torrent := range finished {
		if i == finishedNotificationMaxNames {
			lines = append(lines, fmt.Sprintf("…and %d more", len(finished)-finishedNotificationMaxNames))
			break
		}
		lines = append(lines, torrent.Name)
	}
	return fmt.Sprintf("%d downloads finished", len(finished)), strings.Join(lines, "\n")
}

// noteCompletions feeds a successful poll to the tracker and, when enabled,
// posts one desktop notification for torrents that just finished. The
// tracker runs even while the setting is off, so enabling it later doesn't
// report old completions. Delivery happens off the UI thread; failures are
// only logged.
func (a *application) noteCompletions(torrents []qbt.Torrent) {
	finished := a.completions.observe(torrents)
	if len(finished) == 0 || a.notifier == nil || !a.controller.Config().UI.NotifyOnComplete {
		return
	}
	summary, body := finishedNotificationText(finished)
	notifier := a.notifier
	go func() {
		if err := notifier.Notify(summary, body); err != nil {
			a.logger.Warn("send download finished notification", "count", len(finished), "error", err)
		}
	}()
}

// newNotifier picks the platform notification backend, falling back to a
// no-op one (with a log line) when none is available.
func newNotifier(logger *slog.Logger) notify.Notifier {
	notifier, err := notify.New(appcore.Name)
	if err != nil {
		logger.Warn("desktop notifications unavailable", "error", err)
		return notify.Nop{}
	}
	return notifier
}
