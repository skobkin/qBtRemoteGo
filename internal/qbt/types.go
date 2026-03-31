package qbt

import "time"

type Torrent struct {
	Hash       string    `json:"hash"`
	Name       string    `json:"name"`
	MagnetURI  string    `json:"magnet_uri"`
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

type TorrentProperties struct {
	Name                 string  `json:"name"`
	Hash                 string  `json:"hash"`
	InfoHashV1           string  `json:"infohash_v1"`
	InfoHashV2           string  `json:"infohash_v2"`
	TimeElapsed          int64   `json:"time_elapsed"`
	SeedingTime          int64   `json:"seeding_time"`
	ETASeconds           int64   `json:"eta"`
	Connections          int     `json:"nb_connections"`
	ConnectionLimit      int     `json:"nb_connections_limit"`
	TotalDownloaded      int64   `json:"total_downloaded"`
	SessionDownloaded    int64   `json:"total_downloaded_session"`
	TotalUploaded        int64   `json:"total_uploaded"`
	SessionUploaded      int64   `json:"total_uploaded_session"`
	DownloadSpeed        int64   `json:"dl_speed"`
	AverageDownloadSpeed int64   `json:"dl_speed_avg"`
	UploadSpeed          int64   `json:"up_speed"`
	AverageUploadSpeed   int64   `json:"up_speed_avg"`
	DownloadLimit        int64   `json:"dl_limit"`
	UploadLimit          int64   `json:"up_limit"`
	TotalWasted          int64   `json:"total_wasted"`
	Seeds                int     `json:"seeds"`
	SeedsTotal           int     `json:"seeds_total"`
	Peers                int     `json:"peers"`
	PeersTotal           int     `json:"peers_total"`
	ShareRatio           float64 `json:"share_ratio"`
	Popularity           float64 `json:"popularity"`
	Availability         float64 `json:"availability"`
	ReannounceSeconds    int64   `json:"reannounce"`
	TotalSize            int64   `json:"total_size"`
	PiecesNum            int     `json:"pieces_num"`
	PieceSize            int64   `json:"piece_size"`
	PiecesHave           int     `json:"pieces_have"`
	CreatedBy            string  `json:"created_by"`
	Private              *bool   `json:"private"`
	AdditionDateUnix     int64   `json:"addition_date"`
	LastSeenCompleteUnix int64   `json:"last_seen"`
	CompletionDateUnix   int64   `json:"completion_date"`
	CreationDateUnix     int64   `json:"creation_date"`
	SavePath             string  `json:"save_path"`
	DownloadPath         string  `json:"download_path"`
	Comment              string  `json:"comment"`
	HasMetadata          bool    `json:"has_metadata"`
	Progress             float64 `json:"progress"`
}

type TorrentFile struct {
	Index        int     `json:"index"`
	Name         string  `json:"name"`
	Size         int64   `json:"size"`
	Progress     float64 `json:"progress"`
	Priority     int     `json:"priority"`
	Availability float64 `json:"availability"`
	PieceRange   []int   `json:"piece_range"`
}

type TorrentTracker struct {
	URL              string `json:"url"`
	Tier             int    `json:"tier"`
	Status           int    `json:"status"`
	Message          string `json:"msg"`
	Peers            int    `json:"num_peers"`
	Seeds            int    `json:"num_seeds"`
	Leeches          int    `json:"num_leeches"`
	Downloaded       int    `json:"num_downloaded"`
	Updating         bool   `json:"updating"`
	NextAnnounceUnix int64  `json:"next_announce"`
	MinAnnounceUnix  int64  `json:"min_announce"`
}

type TorrentWebSeed struct {
	URL string `json:"url"`
}

type TorrentPeersSync struct {
	RID          int                    `json:"rid"`
	FullUpdate   bool                   `json:"full_update"`
	ShowFlags    bool                   `json:"show_flags"`
	Peers        map[string]TorrentPeer `json:"peers"`
	PeersRemoved []string               `json:"peers_removed"`
}

type TorrentPeer struct {
	IP               string  `json:"ip"`
	Port             int     `json:"port"`
	Client           string  `json:"client"`
	Progress         float64 `json:"progress"`
	DownloadSpeed    int64   `json:"dl_speed"`
	UploadSpeed      int64   `json:"up_speed"`
	TotalDownloaded  int64   `json:"downloaded"`
	TotalUploaded    int64   `json:"uploaded"`
	Connection       string  `json:"connection"`
	Flags            string  `json:"flags"`
	FlagsDescription string  `json:"flags_desc"`
	Relevance        float64 `json:"relevance"`
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
