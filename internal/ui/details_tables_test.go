package ui

import (
	"testing"

	"github.com/skobkin/qbtremotego/internal/qbt"
)

func TestTrackerStatusLabel(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{status: 0, want: "Disabled"},
		{status: 1, want: "Not contacted"},
		{status: 2, want: "Working"},
		{status: 3, want: "Updating"},
		{status: 4, want: "Not working"},
		{status: 99, want: "99"},
	}

	for _, tt := range tests {
		if got := trackerStatusLabel(tt.status); got != tt.want {
			t.Fatalf("trackerStatusLabel(%d) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestPeerRows(t *testing.T) {
	t.Run("empty peers produce no rows", func(t *testing.T) {
		state := &torrentDetailsState{}
		if rows := peerRows(state); len(rows) != 0 {
			t.Fatalf("expected no rows, got %#v", rows)
		}
	})

	t.Run("peer values are formatted into columns", func(t *testing.T) {
		state := &torrentDetailsState{
			Peers: detailsPeersState{
				Peers: []qbt.TorrentPeer{{
					IP:              "1.2.3.4",
					Port:            6881,
					Connection:      "uTP",
					Flags:           "D",
					Client:          "qBittorrent 5.0",
					Progress:        0.75,
					DownloadSpeed:   1536,
					UploadSpeed:     2048,
					TotalDownloaded: 1024,
					TotalUploaded:   4096,
				}},
			},
		}

		rows := peerRows(state)
		if len(rows) != 1 {
			t.Fatalf("expected one row, got %d", len(rows))
		}
		want := []string{
			"1.2.3.4",
			"6881",
			"uTP",
			"D",
			"qBittorrent 5.0",
			"75.0%",
			"1.5 KiB/s",
			"2.0 KiB/s",
			"1.0 KiB",
			"4.0 KiB",
		}
		for col := range want {
			if rows[0][col] != want[col] {
				t.Fatalf("column %d = %q, want %q", col, rows[0][col], want[col])
			}
		}
	})
}

func TestTrackerRows(t *testing.T) {
	state := &torrentDetailsState{
		Trackers: detailsTrackersState{
			Trackers: []qbt.TorrentTracker{{
				URL:        "udp://tracker.example:1337",
				Tier:       2,
				Status:     2,
				Peers:      11,
				Seeds:      7,
				Leeches:    4,
				Downloaded: 9,
				Message:    "",
			}},
		},
	}

	rows := trackerRows(state)
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	want := []string{
		"2",
		"udp://tracker.example:1337",
		"Working",
		"11",
		"7",
		"4",
		"9",
		"",
	}
	for col := range want {
		if rows[0][col] != want[col] {
			t.Fatalf("column %d = %q, want %q", col, rows[0][col], want[col])
		}
	}
}

func TestWebSeedRows(t *testing.T) {
	t.Run("empty web seeds produce no rows", func(t *testing.T) {
		state := &torrentDetailsState{}
		if rows := webSeedRows(state); len(rows) != 0 {
			t.Fatalf("expected no rows, got %#v", rows)
		}
	})

	t.Run("web seed URLs are formatted into columns", func(t *testing.T) {
		state := &torrentDetailsState{
			WebSeeds: detailsWebSeedsState{
				WebSeeds: []qbt.TorrentWebSeed{
					{URL: "https://seed.example/one"},
					{URL: "https://seed.example/two"},
				},
			},
		}

		rows := webSeedRows(state)
		if len(rows) != 2 {
			t.Fatalf("expected two rows, got %d", len(rows))
		}
		if len(rows[0]) != 1 || rows[0][0] != "https://seed.example/one" {
			t.Fatalf("unexpected row: %#v", rows[0])
		}
		if len(rows[1]) != 1 || rows[1][0] != "https://seed.example/two" {
			t.Fatalf("unexpected row: %#v", rows[1])
		}
	})
}
