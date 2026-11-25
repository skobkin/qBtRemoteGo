package ui

import (
	"testing"

	appcore "github.com/skobkin/qbtremotego/internal/app"
	"github.com/skobkin/qbtremotego/internal/qbt"
)

func seedStateFilterTorrents(app *application) {
	app.allTorrents = []qbt.Torrent{
		{Hash: "h1", Name: "dl", State: "downloading", Progress: 0.3},
		{Hash: "h2", Name: "seed", State: "uploading", Progress: 1},
		{Hash: "h3", Name: "stopped", State: "stoppedUP", Progress: 1},
	}
	app.refreshVisibleTorrents()
}

func TestStateFilterSelectShowsCounts(t *testing.T) {
	app := newTorrentTableTestApp(t)
	seedStateFilterTorrents(app)

	want := []string{"All (3)", "Downloading (1)", "Seeding (1)", "Completed (2)", "Stopped (1)", "Errored (0)"}
	if len(app.stateSelect.Options) != len(want) {
		t.Fatalf("options %v, want %v", app.stateSelect.Options, want)
	}
	for i, label := range want {
		if app.stateSelect.Options[i] != label {
			t.Fatalf("option %d = %q, want %q", i, app.stateSelect.Options[i], label)
		}
	}
	if got := app.stateSelect.SelectedIndex(); got != 0 {
		t.Fatalf("selected index %d, want All", got)
	}
}

func TestStateFilterSelectFiltersAndPersists(t *testing.T) {
	app := newTorrentTableTestApp(t)
	seedStateFilterTorrents(app)

	app.stateSelect.SetSelectedIndex(3) // Completed

	if len(app.visibleTorrents) != 2 {
		t.Fatalf("visible %d torrents, want 2 completed", len(app.visibleTorrents))
	}
	if got := app.controller.Config().UI.StateFilter; got != string(appcore.StateCompleted) {
		t.Fatalf("persisted state filter %q, want completed", got)
	}

	// A poll that changes the counts must keep the chosen bucket.
	app.allTorrents = append(app.allTorrents, qbt.Torrent{Hash: "h4", Name: "done", State: "stalledUP", Progress: 1})
	app.refreshVisibleTorrents()
	if got := app.stateSelect.Selected; got != "Completed (3)" {
		t.Fatalf("selected %q after poll, want Completed (3)", got)
	}
	if len(app.visibleTorrents) != 3 {
		t.Fatalf("visible %d torrents after poll, want 3", len(app.visibleTorrents))
	}
}
