package ui

import (
	"testing"

	"github.com/skobkin/qbtremotego/internal/qbt"
)

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
		Peers:    detailsPeersState{RID: 3, Peers: []qbt.TorrentPeer{{IP: "1.2.3.4"}}},
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
	if state.Peers.RID != 0 || len(state.Peers.Peers) != 0 {
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
