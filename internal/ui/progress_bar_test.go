package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

	"github.com/skobkin/qbtremotego/internal/qbt"
)

func TestTableSpeedTextHidesZero(t *testing.T) {
	tests := []struct {
		speed int64
		want  string
	}{
		{0, ""},
		{-1, ""},
		{1536, "1.5 KiB/s"},
	}
	for _, tt := range tests {
		if got := tableSpeedText(tt.speed); got != tt.want {
			t.Errorf("tableSpeedText(%d) = %q, want %q", tt.speed, got, tt.want)
		}
	}
}

func TestTableETATextHidesInfinity(t *testing.T) {
	tests := []struct {
		seconds int64
		want    string
	}{
		{8640000, ""},
		{0, "Done"},
		{90, "1m"},
	}
	for _, tt := range tests {
		if got := tableETAText(tt.seconds); got != tt.want {
			t.Errorf("tableETAText(%d) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}

func TestProgressFillColorFollowsState(t *testing.T) {
	test.NewTempApp(t)
	primary := theme.Color(theme.ColorNamePrimary)
	tests := []struct {
		name     string
		state    string
		progress float64
		want     any
	}{
		{"downloading uses primary", "downloading", 0.4, primary},
		{"queued download uses primary", "queuedDL", 0.1, primary},
		{"complete uses green", "stalledUP", 1, progressCompleteColor},
		{"seeding uses green", "uploading", 1, progressCompleteColor},
		{"stopped incomplete uses grey", "stoppedDL", 0.3, progressStoppedColor},
		{"stopped complete uses grey", "stoppedUP", 1, progressStoppedColor},
		{"error uses red", "error", 0.5, progressErrorColor},
		{"missing files uses red", "missingFiles", 1, progressErrorColor},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := progressFillColor(tt.state, tt.progress); got != tt.want {
				t.Fatalf("progressFillColor(%q, %.1f) = %v, want %v", tt.state, tt.progress, got, tt.want)
			}
		})
	}
}

func TestStateProgressBarFillWidthFollowsValue(t *testing.T) {
	test.NewTempApp(t)
	bar := newStateProgressBar()
	bar.SetProgress(0.25, progressCompleteColor)
	bar.Resize(fyne.NewSize(200, 24))
	test.WidgetRenderer(bar).Layout(fyne.NewSize(200, 24))

	if got := bar.fill.Size().Width; got != 50 {
		t.Fatalf("fill width %.1f, want 50", got)
	}
	if got := bar.label.Text; got != "25%" {
		t.Fatalf("label %q, want 25%%", got)
	}
	if bar.fill.FillColor != progressCompleteColor {
		t.Fatalf("fill colour %v, want complete colour", bar.fill.FillColor)
	}
}

func TestTorrentRowHidesIdleValuesAndColoursProgress(t *testing.T) {
	app := newTorrentTableTestApp(t)
	row := app.rowForTest(t, "hash-a")

	row.setTorrent(qbt.Torrent{Hash: "hash-a", State: "stoppedUP", Progress: 1, ETASeconds: 8640000})
	if got := row.down.label.Text; got != "" {
		t.Fatalf("idle down speed %q, want empty", got)
	}
	if got := row.up.label.Text; got != "" {
		t.Fatalf("idle up speed %q, want empty", got)
	}
	if got := row.eta.label.Text; got != "" {
		t.Fatalf("infinite ETA %q, want empty", got)
	}
	if row.progress.fill.FillColor != progressStoppedColor {
		t.Fatalf("stopped progress colour %v, want stopped grey", row.progress.fill.FillColor)
	}

	row.setTorrent(qbt.Torrent{Hash: "hash-a", State: "downloading", Progress: 0.5, DLSpeed: 2048, ETASeconds: 90})
	if got := row.down.label.Text; got != "2.0 KiB/s" {
		t.Fatalf("active down speed %q, want 2.0 KiB/s", got)
	}
	if got := row.eta.label.Text; got != "1m" {
		t.Fatalf("ETA %q, want 1m", got)
	}
}
