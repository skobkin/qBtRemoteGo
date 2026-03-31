package ui

import (
	"testing"

	"github.com/skobkin/qbtremotego/internal/qbt"
)

func TestBuildContentTreeAggregatesFolderState(t *testing.T) {
	tree := buildContentTree([]qbt.TorrentFile{
		{
			Name:         "root/keep.bin",
			Size:         100,
			Progress:     0.5,
			Priority:     contentPriorityNormal,
			Availability: 0.8,
		},
		{
			Name:         "root/skip.bin",
			Size:         300,
			Progress:     0.1,
			Priority:     contentPriorityIgnored,
			Availability: 0.2,
		},
		{
			Name:         "root/high.bin",
			Size:         200,
			Progress:     1,
			Priority:     contentPriorityHigh,
			Availability: 1,
		},
	})

	root := tree.root.children[0]
	if root.priority != contentPriorityMixed {
		t.Fatalf("expected mixed priority, got %d", root.priority)
	}
	if root.size != 600 {
		t.Fatalf("unexpected size: %d", root.size)
	}
	if root.remaining != 50 {
		t.Fatalf("unexpected remaining: %d", root.remaining)
	}
	if root.progress < 0.83 || root.progress > 0.84 {
		t.Fatalf("unexpected progress: %f", root.progress)
	}
	if root.availability < 0.93 || root.availability > 0.94 {
		t.Fatalf("unexpected availability: %f", root.availability)
	}
}

func TestContentVisibleRowsFilterIncludesAncestors(t *testing.T) {
	tree := buildContentTree([]qbt.TorrentFile{
		{Name: "folder/child.bin", Size: 10, Progress: 1, Priority: contentPriorityNormal},
		{Name: "other.bin", Size: 20, Progress: 1, Priority: contentPriorityNormal},
	})

	rows := tree.visibleRows("child", map[string]bool{})
	if len(rows) != 2 {
		t.Fatalf("unexpected row count: %d", len(rows))
	}
	if rows[0].node.name != "folder" || rows[1].node.name != "child.bin" {
		t.Fatalf("unexpected filtered rows: %#v %#v", rows[0].node.name, rows[1].node.name)
	}
}

func TestSortedPeersOrdersByAddressKey(t *testing.T) {
	peers := sortedPeers(map[string]qbt.TorrentPeer{
		"b": {IP: "2.2.2.2"},
		"a": {IP: "1.1.1.1"},
	})

	if len(peers) != 2 {
		t.Fatalf("unexpected peer count: %d", len(peers))
	}
	if peers[0].IP != "1.1.1.1" || peers[1].IP != "2.2.2.2" {
		t.Fatalf("unexpected peer order: %#v", peers)
	}
}
