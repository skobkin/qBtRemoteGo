package ui

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
)

func TestDroppedInvocationKeepsTorrentFiles(t *testing.T) {
	dir := t.TempDir()
	torrentPath := filepath.Join(dir, "a.torrent")
	otherPath := filepath.Join(dir, "notes.txt")
	for _, p := range []string{torrentPath, otherPath} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	batch := droppedInvocation([]fyne.URI{storage.NewFileURI(torrentPath), storage.NewFileURI(otherPath)})

	if len(batch.TorrentFiles) != 1 || batch.TorrentFiles[0] != torrentPath {
		t.Fatalf("torrent files %v, want only %s", batch.TorrentFiles, torrentPath)
	}
	if len(batch.MagnetLinks) != 0 {
		t.Fatalf("magnets %v, want none", batch.MagnetLinks)
	}
}
