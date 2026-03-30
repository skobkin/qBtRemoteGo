package app

import (
	"os"
	"strings"

	"github.com/skobkin/qbtremotego/internal/qbt"
)

type AddDialogPrefill struct {
	SourceType      qbt.SourceType
	TorrentFilePath string
	MagnetLinks     []string
}

type InvocationBatch struct {
	MagnetLinks  []string
	TorrentFiles []string
}

func ParseInvocationArgs(args []string) InvocationBatch {
	var batch InvocationBatch

	for _, arg := range args {
		value := strings.TrimSpace(arg)
		if value == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(value), "magnet:") {
			batch.MagnetLinks = append(batch.MagnetLinks, value)
			continue
		}
		if strings.HasSuffix(strings.ToLower(value), ".torrent") {
			if _, err := os.Stat(value); err == nil {
				batch.TorrentFiles = append(batch.TorrentFiles, value)
			}
		}
	}

	return batch
}

func (b InvocationBatch) Empty() bool {
	return len(b.MagnetLinks) == 0 && len(b.TorrentFiles) == 0
}
