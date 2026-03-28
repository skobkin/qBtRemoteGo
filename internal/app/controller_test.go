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
