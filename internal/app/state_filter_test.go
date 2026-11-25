package app

import (
	"testing"

	"github.com/skobkin/qbtremotego/internal/qbt"
)

func TestStateMatches(t *testing.T) {
	tests := []struct {
		state    string
		progress float64
		want     []StateFilter
	}{
		{"downloading", 0.5, []StateFilter{StateAll, StateDownloading}},
		{"forcedDL", 0.5, []StateFilter{StateAll, StateDownloading}},
		{"metaDL", 0, []StateFilter{StateAll, StateDownloading}},
		{"forcedMetaDL", 0, []StateFilter{StateAll, StateDownloading}},
		{"stalledDL", 0.2, []StateFilter{StateAll, StateDownloading}},
		{"queuedDL", 0, []StateFilter{StateAll, StateDownloading}},
		{"checkingDL", 0.3, []StateFilter{StateAll, StateDownloading}},
		{"uploading", 1, []StateFilter{StateAll, StateSeeding, StateCompleted}},
		{"forcedUP", 1, []StateFilter{StateAll, StateSeeding, StateCompleted}},
		{"stalledUP", 1, []StateFilter{StateAll, StateSeeding, StateCompleted}},
		{"queuedUP", 1, []StateFilter{StateAll, StateSeeding, StateCompleted}},
		{"checkingUP", 1, []StateFilter{StateAll, StateSeeding, StateCompleted}},
		{"stoppedUP", 1, []StateFilter{StateAll, StateCompleted, StateStopped}},
		{"pausedUP", 1, []StateFilter{StateAll, StateCompleted, StateStopped}},
		{"stoppedDL", 0.4, []StateFilter{StateAll, StateStopped}},
		{"pausedDL", 0.4, []StateFilter{StateAll, StateStopped}},
		{"error", 0.4, []StateFilter{StateAll, StateErrored}},
		{"missingFiles", 1, []StateFilter{StateAll, StateCompleted, StateErrored}},
	}
	for _, tt := range tests {
		torrent := qbt.Torrent{State: tt.state, Progress: tt.progress}
		want := map[StateFilter]bool{}
		for _, f := range tt.want {
			want[f] = true
		}
		for _, f := range StateFilters() {
			if got := StateMatches(f, torrent); got != want[f] {
				t.Errorf("StateMatches(%q, %s@%.1f) = %v, want %v", f, tt.state, tt.progress, got, want[f])
			}
		}
	}
}

func TestCountByState(t *testing.T) {
	torrents := []qbt.Torrent{
		{State: "downloading", Progress: 0.1},
		{State: "uploading", Progress: 1},
		{State: "stoppedUP", Progress: 1},
		{State: "error"},
	}
	counts := CountByState(torrents)
	want := map[StateFilter]int{
		StateAll: 4, StateDownloading: 1, StateSeeding: 1,
		StateCompleted: 2, StateStopped: 1, StateErrored: 1,
	}
	for f, n := range want {
		if counts[f] != n {
			t.Errorf("count[%q] = %d, want %d", f, counts[f], n)
		}
	}
}

func TestParseStateFilter(t *testing.T) {
	tests := []struct {
		raw  string
		want StateFilter
	}{
		{"", StateAll},
		{"all", StateAll},
		{"Seeding", StateSeeding},
		{" stopped ", StateStopped},
		{"nonsense", StateAll},
		{"downloading", StateDownloading},
	}
	for _, tt := range tests {
		if got := ParseStateFilter(tt.raw); got != tt.want {
			t.Errorf("ParseStateFilter(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestFilterAndSortTorrentsAppliesStateFilter(t *testing.T) {
	items := []qbt.Torrent{
		{Name: "b-seed", State: "uploading", Progress: 1},
		{Name: "a-dl", State: "downloading", Progress: 0.2},
		{Name: "c-seed", State: "stalledUP", Progress: 1},
	}
	got := FilterAndSortTorrents(items, "", "name", StateSeeding, "name", false)
	if len(got) != 2 || got[0].Name != "b-seed" || got[1].Name != "c-seed" {
		t.Fatalf("seeding filter = %+v, want b-seed, c-seed", got)
	}
}

func TestFilterAndSortTorrentsByExtraColumns(t *testing.T) {
	items := []qbt.Torrent{
		{Name: "a", Ratio: 2, NumSeeds: 1, NumLeechs: 9, Category: "b", Tags: "z", SavePath: "/b", Uploaded: 10},
		{Name: "b", Ratio: 1, NumSeeds: 3, NumLeechs: 1, Category: "a", Tags: "y", SavePath: "/a", Uploaded: 30},
	}
	tests := map[string]string{
		"ratio":     "b",
		"seeds":     "a",
		"peers":     "b",
		"category":  "b",
		"tags":      "b",
		"save_path": "b",
		"uploaded":  "a",
	}
	for column, wantFirst := range tests {
		got := FilterAndSortTorrents(items, "", "name", StateAll, column, false)
		if got[0].Name != wantFirst {
			t.Errorf("ascending %s: first %q, want %q", column, got[0].Name, wantFirst)
		}
	}
}
