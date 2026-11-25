package app

import (
	"strings"

	"github.com/skobkin/qbtremotego/internal/qbt"
)

// StateFilter is a quick filter bucket for the torrent list. Buckets overlap
// the way qBittorrent's own sidebar does: a finished, seeding torrent counts
// as both Seeding and Completed.
type StateFilter string

const (
	StateAll         StateFilter = "all"
	StateDownloading StateFilter = "downloading"
	StateSeeding     StateFilter = "seeding"
	StateCompleted   StateFilter = "completed"
	StateStopped     StateFilter = "stopped"
	StateErrored     StateFilter = "errored"
)

// StateFilters lists the buckets in display order.
func StateFilters() []StateFilter {
	return []StateFilter{StateAll, StateDownloading, StateSeeding, StateCompleted, StateStopped, StateErrored}
}

// StateFilterLabel is the human label of a bucket.
func StateFilterLabel(filter StateFilter) string {
	switch filter {
	case StateDownloading:
		return "Downloading"
	case StateSeeding:
		return "Seeding"
	case StateCompleted:
		return "Completed"
	case StateStopped:
		return "Stopped"
	case StateErrored:
		return "Errored"
	default:
		return "All"
	}
}

// ParseStateFilter maps a stored value to a bucket, falling back to All.
func ParseStateFilter(raw string) StateFilter {
	candidate := StateFilter(strings.ToLower(strings.TrimSpace(raw)))
	for _, filter := range StateFilters() {
		if filter == candidate {
			return filter
		}
	}
	return StateAll
}

// StateMatches reports whether a torrent belongs to a bucket.
func StateMatches(filter StateFilter, torrent qbt.Torrent) bool {
	state := strings.ToLower(strings.TrimSpace(torrent.State))
	switch filter {
	case StateAll:
		return true
	case StateDownloading:
		switch state {
		case "downloading", "forceddl", "metadl", "forcedmetadl", "stalleddl", "queueddl", "checkingdl":
			return true
		}
		return false
	case StateSeeding:
		switch state {
		case "uploading", "forcedup", "stalledup", "queuedup", "checkingup":
			return true
		}
		return false
	case StateCompleted:
		return torrent.Progress >= 1
	case StateStopped:
		switch state {
		case "pauseddl", "pausedup", "stoppeddl", "stoppedup":
			return true
		}
		return false
	case StateErrored:
		return state == "error" || state == "missingfiles"
	default:
		return true
	}
}

// CountByState counts the torrents in every bucket.
func CountByState(torrents []qbt.Torrent) map[StateFilter]int {
	counts := make(map[StateFilter]int, len(StateFilters()))
	for _, torrent := range torrents {
		for _, filter := range StateFilters() {
			if StateMatches(filter, torrent) {
				counts[filter]++
			}
		}
	}
	return counts
}
