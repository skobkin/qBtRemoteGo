package app

import (
	"testing"
	"time"

	"github.com/skobkin/qbtremotego/internal/qbt"
)

func TestValidateAddDialogData(t *testing.T) {
	req, err := ValidateAddDialogData(AddDialogData{
		SourceType:        qbt.SourceMagnet,
		MagnetText:        "magnet:?xt=urn:btih:abc\n",
		ManagementMode:    "Auto",
		DownloadLimitText: "0",
		UploadLimitText:   "128",
		ContentLayout:     "Create subfolder",
		StopCondition:     "Files checked",
		StartTorrent:      true,
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(req.MagnetLinks) != 1 || req.ManagementMode != "auto" {
		t.Fatalf("unexpected request: %#v", req)
	}
	if req.ContentLayout != "Subfolder" || req.StopCondition != "FilesChecked" {
		t.Fatalf("unexpected enums: %#v", req)
	}
	if req.UploadLimitKiB == nil || *req.UploadLimitKiB != 128 {
		t.Fatalf("unexpected upload limit: %#v", req.UploadLimitKiB)
	}
}

func TestValidateAddDialogDataRejectsBadRate(t *testing.T) {
	if _, err := ValidateAddDialogData(AddDialogData{
		SourceType:        qbt.SourceMagnet,
		MagnetText:        "magnet:?xt=urn:btih:abc",
		DownloadLimitText: "-1",
	}); err == nil {
		t.Fatal("expected error")
	}
}

func TestFilterAndSortTorrents(t *testing.T) {
	now := time.Now()
	items := []qbt.Torrent{
		{Name: "Zulu", SavePath: "/data/other", AddedUnix: now.Add(-time.Hour).Unix(), AddedAt: now.Add(-time.Hour)},
		{Name: "Alpha", SavePath: "/data/main", AddedUnix: now.Unix(), AddedAt: now},
	}

	filtered := FilterAndSortTorrents(items, "main", "location", "name", false)
	if len(filtered) != 1 || filtered[0].Name != "Alpha" {
		t.Fatalf("unexpected filtered list: %#v", filtered)
	}

	sorted := FilterAndSortTorrents(items, "", "name", "added", true)
	if sorted[0].Name != "Alpha" {
		t.Fatalf("unexpected sorted order: %#v", sorted)
	}
}

func TestStatusLabel(t *testing.T) {
	if StatusLabel("uploading") != "Seeding" {
		t.Fatalf("unexpected label")
	}
	if StatusLabel("missingFiles") != "Missing files" {
		t.Fatalf("unexpected missing files label")
	}
}

func TestHumanAdded(t *testing.T) {
	now := time.Date(2026, time.March, 29, 12, 0, 0, 0, time.UTC)

	cases := map[string]time.Time{
		"2y4m":   now.Add(-(2*365*24 + 4*30*24) * time.Hour),
		"4m10d":  now.Add(-(4*30*24 + 10*24) * time.Hour),
		"15d10h": now.Add(-(15*24 + 10) * time.Hour),
		"10h20m": now.Add(-(10*time.Hour + 20*time.Minute)),
		"12m":    now.Add(-12 * time.Minute),
		"now":    now,
	}

	for want, addedAt := range cases {
		if got := humanElapsed(now, addedAt); got != want {
			t.Fatalf("unexpected human elapsed for %s: got %q", want, got)
		}
	}
}
