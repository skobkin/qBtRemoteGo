package ui

import (
	"context"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/qbt"
)

type detailsPanelMode string

const (
	detailsPanelModeOff          detailsPanelMode = "off"
	detailsPanelModeOverlayRight detailsPanelMode = "overlay_right"
	detailsPanelModeBottomPane   detailsPanelMode = "bottom_pane"
)

type detailsTabKey string

const (
	detailsTabGeneral     detailsTabKey = "general"
	detailsTabContent     detailsTabKey = "content"
	detailsTabPeers       detailsTabKey = "peers"
	detailsTabTrackers    detailsTabKey = "trackers"
	detailsTabHTTPSources detailsTabKey = "http_sources"
)

type detailsDatasetState struct {
	Loading bool
	Loaded  bool
	Error   string
}

type detailsGeneralState struct {
	detailsDatasetState
	Data qbt.TorrentProperties
}

type detailsContentState struct {
	detailsDatasetState
	Files    []qbt.TorrentFile
	Filter   string
	Expanded map[string]bool
}

type detailsPeersState struct {
	detailsDatasetState
	RID   int
	Peers []qbt.TorrentPeer
}

type detailsTrackersState struct {
	detailsDatasetState
	Trackers []qbt.TorrentTracker
}

type detailsWebSeedsState struct {
	detailsDatasetState
	WebSeeds []qbt.TorrentWebSeed
}

type torrentDetailsState struct {
	Visible     bool
	FocusedHash string
	ActiveTab   detailsTabKey
	General     detailsGeneralState
	Content     detailsContentState
	Peers       detailsPeersState
	Trackers    detailsTrackersState
	WebSeeds    detailsWebSeedsState
}

func newTorrentDetailsState() *torrentDetailsState {
	return &torrentDetailsState{
		ActiveTab: detailsTabGeneral,
		Content: detailsContentState{
			Expanded: map[string]bool{},
		},
	}
}

// activeDataset returns the load-state embedded in the given tab's dataset, or
// nil for an unknown tab.
func (s *torrentDetailsState) activeDataset(tab detailsTabKey) *detailsDatasetState {
	if s == nil {
		return nil
	}
	switch tab {
	case detailsTabGeneral:
		return &s.General.detailsDatasetState
	case detailsTabContent:
		return &s.Content.detailsDatasetState
	case detailsTabPeers:
		return &s.Peers.detailsDatasetState
	case detailsTabTrackers:
		return &s.Trackers.detailsDatasetState
	case detailsTabHTTPSources:
		return &s.WebSeeds.detailsDatasetState
	default:
		return nil
	}
}

func (s *torrentDetailsState) resetForHash(hash string) {
	if s == nil {
		return
	}
	s.FocusedHash = strings.TrimSpace(hash)
	s.ActiveTab = detailsTabGeneral
	s.General = detailsGeneralState{}
	s.Content = detailsContentState{Expanded: map[string]bool{}}
	s.Peers = detailsPeersState{}
	s.Trackers = detailsTrackersState{}
	s.WebSeeds = detailsWebSeedsState{}
}

func (s *torrentDetailsState) setActiveTab(tab detailsTabKey) {
	if s == nil {
		return
	}
	switch tab {
	case detailsTabGeneral, detailsTabContent, detailsTabPeers, detailsTabTrackers, detailsTabHTTPSources:
		s.ActiveTab = tab
	default:
		s.ActiveTab = detailsTabGeneral
	}
}

func detailsModeFromConfig(cfg config.UIConfig) detailsPanelMode {
	if !cfg.DetailsPanelEnabled {
		return detailsPanelModeOff
	}
	switch strings.ToLower(strings.TrimSpace(cfg.DetailsPanelMode)) {
	case string(detailsPanelModeOverlayRight):
		return detailsPanelModeOverlayRight
	case string(detailsPanelModeBottomPane):
		return detailsPanelModeBottomPane
	default:
		return detailsPanelModeOff
	}
}

func (a *application) currentDetailsMode() detailsPanelMode {
	return detailsModeFromConfig(a.controller.Config().UI)
}

func (a *application) activeDetailsHash() string {
	if a.detailsState == nil {
		return ""
	}
	return strings.TrimSpace(a.detailsState.FocusedHash)
}

func (a *application) selectedSingleHash() string {
	hashes := a.selectedHashes()
	if len(hashes) != 1 {
		return ""
	}
	return hashes[0]
}

func (a *application) ensureDetailsFocusForSelection() {
	if a.detailsState == nil {
		return
	}
	switch a.currentDetailsMode() {
	case detailsPanelModeBottomPane:
		hash := a.selectedSingleHash()
		if hash == "" {
			a.detailsState.Visible = false
			a.detailsState.FocusedHash = ""
			a.refreshDetailsPresentation()
			return
		}
		if a.detailsState.FocusedHash != hash {
			a.detailsState.resetForHash(hash)
		}
		a.detailsState.Visible = true
		a.refreshDetailsPresentation()
		a.ensureActiveDetailsLoaded()
	case detailsPanelModeOverlayRight:
		hash := a.activeDetailsHash()
		if hash == "" {
			return
		}
		if _, ok := a.findTorrentByHash(hash); !ok {
			a.closeTorrentDetails()
		}
	default:
		a.closeTorrentDetails()
	}
}

func (a *application) openTorrentDetails(hash string) {
	if a.detailsState == nil {
		a.detailsState = newTorrentDetailsState()
	}
	hash = strings.TrimSpace(hash)
	if hash == "" || a.currentDetailsMode() == detailsPanelModeOff {
		return
	}
	if a.detailsState.FocusedHash != hash {
		a.detailsState.resetForHash(hash)
	}
	a.detailsState.Visible = true
	a.refreshDetailsPresentation()
	a.ensureActiveDetailsLoaded()
}

func (a *application) closeTorrentDetails() {
	if a.detailsState == nil {
		return
	}
	a.detailsState.Visible = false
	a.refreshDetailsPresentation()
}

func (a *application) setDetailsTab(tab detailsTabKey) {
	if a.detailsState == nil {
		return
	}
	a.detailsState.setActiveTab(tab)
	a.refreshDetailsPresentation()
	a.ensureActiveDetailsLoaded()
}

func (a *application) refreshDetailsPresentation() {
	if a.detailsHost != nil {
		a.detailsHost.Refresh()
	}
}

func (a *application) ensureActiveDetailsLoaded() {
	if a.detailsState == nil {
		return
	}
	hash := a.activeDetailsHash()
	if hash == "" {
		return
	}
	if a.currentDetailsMode() == detailsPanelModeOff {
		return
	}
	if !a.detailsState.Visible && a.currentDetailsMode() != detailsPanelModeBottomPane {
		return
	}
	switch a.detailsState.ActiveTab {
	case detailsTabGeneral:
		if !a.detailsState.General.Loaded && !a.detailsState.General.Loading {
			a.loadTorrentDetailsGeneral(hash)
		}
	case detailsTabContent:
		if !a.detailsState.Content.Loaded && !a.detailsState.Content.Loading {
			a.loadTorrentDetailsContent(hash)
		}
	case detailsTabPeers:
		if !a.detailsState.Peers.Loaded && !a.detailsState.Peers.Loading {
			a.loadTorrentDetailsPeers(hash, 0)
		}
	case detailsTabTrackers:
		if !a.detailsState.Trackers.Loaded && !a.detailsState.Trackers.Loading {
			a.loadTorrentDetailsTrackers(hash)
		}
	case detailsTabHTTPSources:
		if !a.detailsState.WebSeeds.Loaded && !a.detailsState.WebSeeds.Loading {
			a.loadTorrentDetailsWebSeeds(hash)
		}
	}
}

func (a *application) refreshActiveDetails() {
	if a.detailsState == nil {
		return
	}
	hash := a.activeDetailsHash()
	if hash == "" {
		return
	}
	if a.currentDetailsMode() == detailsPanelModeOff {
		return
	}
	if !a.detailsState.Visible && a.currentDetailsMode() != detailsPanelModeBottomPane {
		return
	}
	switch a.detailsState.ActiveTab {
	case detailsTabGeneral:
		if a.detailsState.General.Loading {
			return
		}
		a.loadTorrentDetailsGeneral(hash)
	case detailsTabContent:
		if a.detailsState.Content.Loaded && !a.detailsState.Content.Loading {
			a.loadTorrentDetailsContent(hash)
		}
	case detailsTabPeers:
		if a.detailsState.Peers.Loaded && !a.detailsState.Peers.Loading {
			a.loadTorrentDetailsPeers(hash, 0)
		}
	case detailsTabTrackers:
		if a.detailsState.Trackers.Loaded && !a.detailsState.Trackers.Loading {
			a.loadTorrentDetailsTrackers(hash)
		}
	case detailsTabHTTPSources:
		if a.detailsState.WebSeeds.Loaded && !a.detailsState.WebSeeds.Loading {
			a.loadTorrentDetailsWebSeeds(hash)
		}
	}
}

// detailsLoadTimeout bounds a single details dataset fetch.
const detailsLoadTimeout = 20 * time.Second

// loadDetailsDataset performs the shared details fetch flow for one tab: flag
// the dataset as loading, fetch it off the UI thread, and store the result on
// the UI thread unless the focused torrent changed in the meantime. apply
// receives the fetched data only on success and must only write tab-specific
// fields; the embedded detailsDatasetState is managed here.
func loadDetailsDataset[T any](
	a *application,
	hash string,
	tab detailsTabKey,
	fetch func(ctx context.Context, hash string) (T, error),
	apply func(s *torrentDetailsState, data T),
) {
	dataset := a.detailsState.activeDataset(tab)
	if dataset == nil {
		return
	}
	dataset.Loading = true
	a.refreshDetailsPresentation()
	go func(expected string) {
		ctx, cancel := context.WithTimeout(context.Background(), detailsLoadTimeout)
		defer cancel()
		data, err := fetch(ctx, expected)
		fyne.Do(func() {
			if a.detailsState == nil || a.activeDetailsHash() != expected {
				return
			}
			dataset := a.detailsState.activeDataset(tab)
			if dataset == nil {
				return
			}
			dataset.Loading = false
			dataset.Loaded = err == nil
			if err != nil {
				dataset.Error = err.Error()
			} else {
				dataset.Error = ""
				if apply != nil {
					apply(a.detailsState, data)
				}
			}
			a.refreshDetailsPresentation()
		})
	}(hash)
}

func (a *application) loadTorrentDetailsGeneral(hash string) {
	loadDetailsDataset(a, hash, detailsTabGeneral,
		func(ctx context.Context, hash string) (qbt.TorrentProperties, error) {
			return a.controller.FetchTorrentProperties(ctx, hash)
		},
		func(s *torrentDetailsState, data qbt.TorrentProperties) {
			s.General.Data = data
		})
}

func (a *application) loadTorrentDetailsContent(hash string) {
	loadDetailsDataset(a, hash, detailsTabContent,
		func(ctx context.Context, hash string) ([]qbt.TorrentFile, error) {
			return a.controller.FetchTorrentFiles(ctx, hash)
		},
		func(s *torrentDetailsState, files []qbt.TorrentFile) {
			s.Content.Files = files
			if s.Content.Expanded == nil {
				s.Content.Expanded = map[string]bool{}
			}
		})
}

func (a *application) loadTorrentDetailsPeers(hash string, rid int) {
	loadDetailsDataset(a, hash, detailsTabPeers,
		func(ctx context.Context, hash string) (qbt.TorrentPeersSync, error) {
			return a.controller.FetchTorrentPeers(ctx, hash, rid)
		},
		func(s *torrentDetailsState, data qbt.TorrentPeersSync) {
			s.Peers.RID = data.RID
			s.Peers.Peers = sortedPeers(data.Peers)
		})
}

func (a *application) loadTorrentDetailsTrackers(hash string) {
	loadDetailsDataset(a, hash, detailsTabTrackers,
		func(ctx context.Context, hash string) ([]qbt.TorrentTracker, error) {
			return a.controller.FetchTorrentTrackers(ctx, hash)
		},
		func(s *torrentDetailsState, data []qbt.TorrentTracker) {
			s.Trackers.Trackers = data
		})
}

func (a *application) loadTorrentDetailsWebSeeds(hash string) {
	loadDetailsDataset(a, hash, detailsTabHTTPSources,
		func(ctx context.Context, hash string) ([]qbt.TorrentWebSeed, error) {
			return a.controller.FetchTorrentWebSeeds(ctx, hash)
		},
		func(s *torrentDetailsState, data []qbt.TorrentWebSeed) {
			s.WebSeeds.WebSeeds = data
		})
}

func sortedPeers(in map[string]qbt.TorrentPeer) []qbt.TorrentPeer {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]qbt.TorrentPeer, 0, len(keys))
	for _, key := range keys {
		out = append(out, in[key])
	}
	return out
}

func (a *application) setContentFilter(value string) {
	if a.detailsState == nil {
		return
	}
	a.detailsState.Content.Filter = strings.TrimSpace(value)
	a.refreshDetailsPresentation()
}

func (a *application) toggleContentNode(path string) {
	if a.detailsState == nil || strings.TrimSpace(path) == "" {
		return
	}
	if a.detailsState.Content.Expanded == nil {
		a.detailsState.Content.Expanded = map[string]bool{}
	}
	a.detailsState.Content.Expanded[path] = !a.detailsState.Content.Expanded[path]
	a.refreshDetailsPresentation()
}
