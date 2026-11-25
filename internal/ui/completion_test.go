package ui

import (
	"testing"
	"time"

	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/qbt"
)

func names(torrents []qbt.Torrent) []string {
	out := make([]string, 0, len(torrents))
	for _, torrent := range torrents {
		out = append(out, torrent.Name)
	}
	return out
}

func TestCompletionTrackerReportsOnlyNewlyFinished(t *testing.T) {
	var tracker completionTracker

	first := []qbt.Torrent{{Hash: "a", Name: "a", Progress: 0.5}, {Hash: "b", Name: "b", Progress: 1}}
	if got := tracker.observe(first); len(got) != 0 {
		t.Fatalf("first poll reported %v, want a silent baseline", names(got))
	}

	second := []qbt.Torrent{
		{Hash: "a", Name: "a", Progress: 1},   // finished since last poll
		{Hash: "b", Name: "b", Progress: 1},   // was already complete
		{Hash: "c", Name: "c", Progress: 1},   // new and already complete
		{Hash: "d", Name: "d", Progress: 0.1}, // new and incomplete
	}
	got := tracker.observe(second)
	if len(got) != 1 || got[0].Hash != "a" {
		t.Fatalf("second poll reported %v, want [a]", names(got))
	}

	third := []qbt.Torrent{{Hash: "d", Name: "d", Progress: 1}}
	if got := tracker.observe(third); len(got) != 1 || got[0].Hash != "d" {
		t.Fatalf("third poll reported %v, want [d]", names(got))
	}
}

func TestCompletionTrackerResetRebaselines(t *testing.T) {
	var tracker completionTracker
	tracker.observe([]qbt.Torrent{{Hash: "a", Progress: 0.5}})

	tracker.reset()

	if got := tracker.observe([]qbt.Torrent{{Hash: "a", Progress: 1}}); len(got) != 0 {
		t.Fatalf("first poll after reset reported %v, want a silent baseline", names(got))
	}
}

func TestFinishedNotificationText(t *testing.T) {
	summary, body := finishedNotificationText([]qbt.Torrent{{Name: "debian.iso"}})
	if summary != "Download finished" || body != "debian.iso" {
		t.Fatalf("single = (%q, %q)", summary, body)
	}

	summary, body = finishedNotificationText([]qbt.Torrent{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}})
	if summary != "4 downloads finished" || body != "a\nb\nc\n…and 1 more" {
		t.Fatalf("several = (%q, %q)", summary, body)
	}
}

type recordingNotifier struct {
	sent chan [2]string
}

func (n *recordingNotifier) Notify(summary, body string) error {
	n.sent <- [2]string{summary, body}
	return nil
}

func TestNoteCompletionsNotifiesOnlyWhenEnabled(t *testing.T) {
	app := newTorrentTableTestApp(t)
	notifier := &recordingNotifier{sent: make(chan [2]string, 4)}
	app.notifier = notifier

	app.noteCompletions([]qbt.Torrent{{Hash: "a", Name: "a", Progress: 0.5}})
	app.noteCompletions([]qbt.Torrent{{Hash: "a", Name: "a", Progress: 1}})
	select {
	case msg := <-notifier.sent:
		t.Fatalf("notified %v while the setting is off", msg)
	case <-time.After(50 * time.Millisecond):
	}

	if err := app.controller.SaveLocalUI(func(cfg *config.AppConfig) { cfg.UI.NotifyOnComplete = true }); err != nil {
		t.Fatalf("save config: %v", err)
	}
	app.noteCompletions([]qbt.Torrent{{Hash: "a", Name: "a", Progress: 1}, {Hash: "b", Name: "b", Progress: 0.2}})
	app.noteCompletions([]qbt.Torrent{{Hash: "a", Name: "a", Progress: 1}, {Hash: "b", Name: "b", Progress: 1}})
	select {
	case msg := <-notifier.sent:
		if msg[1] != "b" {
			t.Fatalf("notification %v, want b", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("no notification for a torrent that finished while enabled")
	}
}
