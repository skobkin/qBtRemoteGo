package ui

import (
	"context"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/qbt"
)

func TestDetailsModeFromConfig(t *testing.T) {
	tests := []struct {
		name       string
		enabled    bool
		storedMode string
		want       detailsPanelMode
	}{
		{name: "disabled means off", enabled: false, storedMode: "overlay_right", want: detailsPanelModeOff},
		{name: "enabled overlay", enabled: true, storedMode: "overlay_right", want: detailsPanelModeOverlayRight},
		{name: "enabled bottom pane", enabled: true, storedMode: "bottom_pane", want: detailsPanelModeBottomPane},
		{name: "enabled with stored off falls back to pane", enabled: true, storedMode: "off", want: detailsPanelModeBottomPane},
		{name: "enabled with unknown value falls back to pane", enabled: true, storedMode: "nonsense", want: detailsPanelModeBottomPane},
		{name: "enabled with empty value falls back to pane", enabled: true, storedMode: "", want: detailsPanelModeBottomPane},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.UIConfig{DetailsPanelEnabled: tt.enabled, DetailsPanelMode: tt.storedMode}
			if got := detailsModeFromConfig(cfg); got != tt.want {
				t.Fatalf("detailsModeFromConfig(enabled=%v, mode=%q) = %q, want %q", tt.enabled, tt.storedMode, got, tt.want)
			}
		})
	}
}

func TestDetailsModeSelectLabelRoundTrip(t *testing.T) {
	tests := []struct {
		mode  detailsPanelMode
		label string
	}{
		{mode: detailsPanelModeOverlayRight, label: "Right overlay"},
		{mode: detailsPanelModeBottomPane, label: "Bottom pane"},
		{mode: detailsPanelModeOff, label: "Bottom pane"},
		{mode: detailsPanelMode("junk"), label: "Bottom pane"},
	}

	for _, tt := range tests {
		t.Run(tt.label+"/"+string(tt.mode), func(t *testing.T) {
			if got := detailsModeSelectLabel(tt.mode); got != tt.label {
				t.Fatalf("detailsModeSelectLabel(%q) = %q, want %q", tt.mode, got, tt.label)
			}
			if got := detailsModeFromLabel(tt.label); got == detailsPanelModeOff {
				t.Fatalf("detailsModeFromLabel(%q) returned off; labels must be concrete", tt.label)
			}
		})
	}

	t.Run("label round trip is stable", func(t *testing.T) {
		for _, mode := range []detailsPanelMode{detailsPanelModeOverlayRight, detailsPanelModeBottomPane} {
			if got := detailsModeFromLabel(detailsModeSelectLabel(mode)); got != mode {
				t.Fatalf("round trip of %q produced %q", mode, got)
			}
		}
	})

	t.Run("unknown label falls back to pane", func(t *testing.T) {
		if got := detailsModeFromLabel("Disabled"); got != detailsPanelModeBottomPane {
			t.Fatalf("detailsModeFromLabel(Disabled) = %q, want bottom pane", got)
		}
		if got := detailsModeFromLabel(""); got != detailsPanelModeBottomPane {
			t.Fatalf("detailsModeFromLabel(empty) = %q, want bottom pane", got)
		}
	})
}

func TestResetForHash(t *testing.T) {
	state := &torrentDetailsState{
		Visible:     true,
		FocusedHash: "old",
		ActiveTab:   detailsTabPeers,
		General:     detailsGeneralState{Data: qbt.TorrentProperties{Name: "demo"}},
		Content: detailsContentState{
			Files:  []qbt.TorrentFile{{Name: "a.bin"}},
			Filter: "abc",
			Expanded: map[string]bool{
				"dir": true,
			},
		},
		Peers:    detailsPeersState{Peers: []qbt.TorrentPeer{{IP: "1.2.3.4"}}},
		Trackers: detailsTrackersState{Trackers: []qbt.TorrentTracker{{URL: "udp://tracker"}}},
		WebSeeds: detailsWebSeedsState{WebSeeds: []qbt.TorrentWebSeed{{URL: "https://seed.example"}}},
	}

	state.resetForHash("  new  ")

	if state.FocusedHash != "new" {
		t.Fatalf("expected trimmed focused hash, got %q", state.FocusedHash)
	}
	if state.ActiveTab != detailsTabGeneral {
		t.Fatalf("expected active tab to reset to general, got %q", state.ActiveTab)
	}
	if !state.Visible {
		t.Fatal("expected visibility to be preserved on hash reset")
	}
	if state.General.Data.Name != "" {
		t.Fatalf("expected general data to reset, got %#v", state.General.Data)
	}
	if len(state.Content.Files) != 0 || state.Content.Filter != "" {
		t.Fatalf("expected content dataset to reset, got files=%d filter=%q", len(state.Content.Files), state.Content.Filter)
	}
	if state.Content.Expanded == nil || len(state.Content.Expanded) != 0 {
		t.Fatalf("expected a fresh empty expansion map, got %#v", state.Content.Expanded)
	}
	if len(state.Peers.Peers) != 0 {
		t.Fatalf("expected peers dataset to reset, got %#v", state.Peers)
	}
	if len(state.Trackers.Trackers) != 0 {
		t.Fatalf("expected trackers dataset to reset, got %#v", state.Trackers)
	}
	if len(state.WebSeeds.WebSeeds) != 0 {
		t.Fatalf("expected web seeds dataset to reset, got %#v", state.WebSeeds)
	}
}

func TestResetForHashNilReceiver(t *testing.T) {
	var state *torrentDetailsState
	state.resetForHash("abc")
}

func TestActiveDataset(t *testing.T) {
	state := newTorrentDetailsState()

	tests := []struct {
		name  string
		tab   detailsTabKey
		check func() *detailsDatasetState
	}{
		{name: "general", tab: detailsTabGeneral, check: func() *detailsDatasetState { return &state.General.detailsDatasetState }},
		{name: "content", tab: detailsTabContent, check: func() *detailsDatasetState { return &state.Content.detailsDatasetState }},
		{name: "peers", tab: detailsTabPeers, check: func() *detailsDatasetState { return &state.Peers.detailsDatasetState }},
		{name: "trackers", tab: detailsTabTrackers, check: func() *detailsDatasetState { return &state.Trackers.detailsDatasetState }},
		{name: "http sources", tab: detailsTabHTTPSources, check: func() *detailsDatasetState { return &state.WebSeeds.detailsDatasetState }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := state.activeDataset(tt.tab)
			if got == nil {
				t.Fatalf("activeDataset(%q) returned nil", tt.tab)
			}
			if got != tt.check() {
				t.Fatalf("activeDataset(%q) returned a different pointer than the tab's embedded state", tt.tab)
			}
		})
	}

	t.Run("unknown tab returns nil", func(t *testing.T) {
		if got := state.activeDataset(detailsTabKey("nope")); got != nil {
			t.Fatalf("expected nil for unknown tab, got %#v", got)
		}
	})

	t.Run("nil receiver returns nil", func(t *testing.T) {
		var nilState *torrentDetailsState
		if got := nilState.activeDataset(detailsTabGeneral); got != nil {
			t.Fatalf("expected nil for nil receiver, got %#v", got)
		}
	})
}

func TestDetailsShouldEnsureLoad(t *testing.T) {
	tests := []struct {
		name   string
		state  detailsDatasetState
		ensure bool
	}{
		{name: "fresh dataset needs a load", state: detailsDatasetState{}, ensure: true},
		{name: "loading dataset does not re-load", state: detailsDatasetState{Loading: true}, ensure: false},
		{name: "loaded dataset does not re-load", state: detailsDatasetState{Loaded: true}, ensure: false},
		{name: "errored dataset waits for explicit retry", state: detailsDatasetState{Error: "boom"}, ensure: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detailsShouldEnsureLoad(&tt.state); got != tt.ensure {
				t.Fatalf("detailsShouldEnsureLoad(%#v) = %v, want %v", tt.state, got, tt.ensure)
			}
		})
	}

	if detailsShouldEnsureLoad(nil) {
		t.Fatal("detailsShouldEnsureLoad(nil) = true, want false")
	}
}

func TestDetailsShouldRefreshLoad(t *testing.T) {
	tests := []struct {
		name    string
		state   detailsDatasetState
		refresh bool
	}{
		{name: "fresh dataset has nothing to refresh", state: detailsDatasetState{}, refresh: false},
		{name: "loading dataset does not refresh", state: detailsDatasetState{Loading: true}, refresh: false},
		{name: "loaded dataset refreshes", state: detailsDatasetState{Loaded: true}, refresh: true},
		{name: "errored dataset never auto-refreshes", state: detailsDatasetState{Error: "boom"}, refresh: false},
		{name: "loaded and in-flight does not refresh", state: detailsDatasetState{Loaded: true, Loading: true}, refresh: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detailsShouldRefreshLoad(&tt.state); got != tt.refresh {
				t.Fatalf("detailsShouldRefreshLoad(%#v) = %v, want %v", tt.state, got, tt.refresh)
			}
		})
	}

	if detailsShouldRefreshLoad(nil) {
		t.Fatal("detailsShouldRefreshLoad(nil) = true, want false")
	}
}

func TestSetActiveTab(t *testing.T) {
	tests := []struct {
		name string
		tab  detailsTabKey
		want detailsTabKey
	}{
		{name: "general", tab: detailsTabGeneral, want: detailsTabGeneral},
		{name: "content", tab: detailsTabContent, want: detailsTabContent},
		{name: "peers", tab: detailsTabPeers, want: detailsTabPeers},
		{name: "trackers", tab: detailsTabTrackers, want: detailsTabTrackers},
		{name: "http sources", tab: detailsTabHTTPSources, want: detailsTabHTTPSources},
		{name: "unknown falls back to general", tab: detailsTabKey("nope"), want: detailsTabGeneral},
		{name: "empty falls back to general", tab: detailsTabKey(""), want: detailsTabGeneral},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &torrentDetailsState{ActiveTab: detailsTabPeers}
			state.setActiveTab(tt.tab)
			if state.ActiveTab != tt.want {
				t.Fatalf("setActiveTab(%q) = %q, want %q", tt.tab, state.ActiveTab, tt.want)
			}
		})
	}
}

func TestLoadDetailsDatasetSettlesCurrentFetch(t *testing.T) {
	test.NewTempApp(t)

	app := &application{detailsState: newTorrentDetailsState()}
	const hash = "abc123"
	app.detailsState.resetForHash(hash)

	applied := make(chan struct{})
	loadDetailsDataset(app, hash, detailsTabGeneral,
		func(_ context.Context, _ string) (qbt.TorrentProperties, error) {
			return qbt.TorrentProperties{Name: "fresh"}, nil
		},
		func(s *torrentDetailsState, data qbt.TorrentProperties) {
			s.General.Data = data
			close(applied)
		})

	select {
	case <-applied:
	case <-time.After(2 * time.Second):
		t.Fatal("a current fetch never settled its dataset")
	}

	dataset := app.detailsState.activeDataset(detailsTabGeneral)
	if dataset == nil || !dataset.Loaded || dataset.Loading || dataset.Error != "" {
		t.Fatalf("dataset was not settled after a successful fetch: %#v", dataset)
	}
	if app.detailsState.General.Data.Name != "fresh" {
		t.Fatalf("unexpected applied data: %#v", app.detailsState.General.Data)
	}
}

func TestLoadDetailsDatasetIgnoresFetchSupersededByReselect(t *testing.T) {
	test.NewTempApp(t)

	app := &application{detailsState: newTorrentDetailsState()}
	const hash = "abc123"
	app.detailsState.resetForHash(hash)

	fetchStarted := make(chan struct{})
	release := make(chan error)
	applied := make(chan struct{})
	fetch := func(_ context.Context, h string) (qbt.TorrentProperties, error) {
		if h != hash {
			t.Errorf("fetch called with hash %q, want %q", h, hash)
		}
		close(fetchStarted)
		return qbt.TorrentProperties{Name: "stale"}, <-release
	}
	apply := func(s *torrentDetailsState, data qbt.TorrentProperties) {
		s.General.Data = data
		close(applied)
	}

	loadDetailsDataset(app, hash, detailsTabGeneral, fetch, apply)
	<-fetchStarted

	// Reselecting the same torrent resets the datasets without changing the
	// hash, so only the dataset replacement itself may stop the in-flight
	// fetch from settling what is now a different struct.
	app.detailsState.resetForHash(hash)

	close(release)

	// The superseded callback may still be in flight; give it a moment to
	// (wrongly) land before asserting that it did not.
	select {
	case <-applied:
		t.Fatal("a fetch superseded by a reselect settled the replacement dataset")
	case <-time.After(100 * time.Millisecond):
	}

	dataset := app.detailsState.activeDataset(detailsTabGeneral)
	if dataset == nil || dataset.Loading || dataset.Loaded || dataset.Error != "" {
		t.Fatalf("replacement dataset was touched by the stale fetch: %#v", dataset)
	}
	if app.detailsState.General.Data.Name != "" {
		t.Fatalf("stale data landed in the replacement dataset: %q", app.detailsState.General.Data.Name)
	}
}
