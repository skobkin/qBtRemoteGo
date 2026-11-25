package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	appcore "github.com/skobkin/qbtremotego/internal/app"
	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/qbt"
)

func TestEmptyStateKind(t *testing.T) {
	tests := []struct {
		name       string
		needsSetup bool
		state      connectionState
		total      int
		visible    int
		want       emptyStateKind
	}{
		{"rows visible", false, connectionStateConnected, 3, 3, emptyStateNone},
		{"stale rows while disconnected", false, connectionStateDisconnected, 3, 3, emptyStateNone},
		{"not configured", true, connectionStateUnknown, 0, 0, emptyStateNotConnected},
		{"disconnected and empty", false, connectionStateDisconnected, 0, 0, emptyStateNotConnected},
		{"first refresh pending", false, connectionStateUnknown, 0, 0, emptyStateNone},
		{"filters hide everything", false, connectionStateConnected, 3, 0, emptyStateNoMatches},
		{"server has no torrents", false, connectionStateConnected, 0, 0, emptyStateNoTorrents},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := emptyStateFor(tt.needsSetup, tt.state, tt.total, tt.visible); got != tt.want {
				t.Fatalf("emptyStateFor = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEmptyStateClearFiltersResetsSearchAndState(t *testing.T) {
	app := newTorrentTableTestApp(t)
	app.connectionState = connectionStateConnected
	app.filterEntry.SetText("zzz")
	app.applyFilterQueryNow("zzz")
	app.setStateFilter(appcore.StateSeeding)

	if !app.emptyState.Visible() {
		t.Fatal("empty state hidden while filters hide every torrent")
	}
	if got := app.emptyStateMessage.Text; got != "No torrents match the current filters" {
		t.Fatalf("message %q, want no-match message", got)
	}

	test.Tap(app.emptyStateAction)

	if app.filterQuery != "" || app.filterEntry.Text != "" {
		t.Fatalf("search %q/%q after clear, want empty", app.filterQuery, app.filterEntry.Text)
	}
	if app.stateFilter != appcore.StateAll {
		t.Fatalf("state filter %q after clear, want all", app.stateFilter)
	}
	if app.emptyState.Visible() {
		t.Fatal("empty state still visible after clearing filters")
	}
}

func TestEmptyStateShowsNoTorrentsWhenServerIsEmpty(t *testing.T) {
	app := newTorrentTableTestApp(t)
	if err := app.controller.SaveLocalUI(func(cfg *config.AppConfig) {
		cfg.Connection.URL = "http://qbt.invalid"
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	app.connectionState = connectionStateConnected
	app.allTorrents = []qbt.Torrent{}
	app.refreshVisibleTorrents()

	if !app.emptyState.Visible() {
		t.Fatal("empty state hidden for an empty server")
	}
	if got := app.emptyStateAction.Text; got != "Add torrent" {
		t.Fatalf("action %q, want Add torrent", got)
	}
}
