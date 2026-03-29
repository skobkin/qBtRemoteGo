package qbt

import "time"

type Torrent struct {
	Hash       string    `json:"hash"`
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	Progress   float64   `json:"progress"`
	State      string    `json:"state"`
	SavePath   string    `json:"save_path"`
	DLSpeed    int64     `json:"dlspeed"`
	UPSpeed    int64     `json:"upspeed"`
	ETASeconds int64     `json:"eta"`
	AddedUnix  int64     `json:"added_on"`
	AddedAt    time.Time `json:"-"`
}

type TransferInfo struct {
	DownloadSpeed    int64  `json:"dl_info_speed"`
	UploadSpeed      int64  `json:"up_info_speed"`
	DownloadLimit    int64  `json:"dl_rate_limit"`
	UploadLimit      int64  `json:"up_rate_limit"`
	ConnectionStatus string `json:"connection_status"`
}

type MainData struct {
	ServerState ServerState `json:"server_state"`
}

type ServerState struct {
	FreeSpaceOnDisk   int64 `json:"free_space_on_disk"`
	UseAltSpeedLimits bool  `json:"use_alt_speed_limits"`
}

type AddRequest struct {
	SourceType          SourceType
	TorrentFilePath     string
	MagnetLinks         []string
	ManagementMode      string
	SavePath            string
	Rename              string
	Category            string
	Tags                []string
	StartTorrent        bool
	TopOfQueue          bool
	StopCondition       string
	SkipHashCheck       bool
	ContentLayout       string
	SequentialDownload  bool
	FirstLastPieceFirst bool
	DownloadLimitKiB    *int
	UploadLimitKiB      *int
}

type SourceType string

const (
	SourceTorrentFile SourceType = "torrent"
	SourceMagnet      SourceType = "magnet"
)
