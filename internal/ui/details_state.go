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

func (a *application) loadTorrentDetailsGeneral(hash string) {
	a.detailsState.General.Loading = true
	a.refreshDetailsPresentation()
	go func(expected string) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		data, err := a.controller.FetchTorrentProperties(ctx, expected)
		fyne.Do(func() {
			if a.activeDetailsHash() != expected || a.detailsState == nil {
				return
			}
			a.detailsState.General.Loading = false
			a.detailsState.General.Loaded = err == nil
			if err != nil {
				a.detailsState.General.Error = err.Error()
			} else {
				a.detailsState.General.Error = ""
				a.detailsState.General.Data = data
			}
			a.refreshDetailsPresentation()
		})
	}(hash)
}

func (a *application) loadTorrentDetailsContent(hash string) {
	a.detailsState.Content.Loading = true
	a.refreshDetailsPresentation()
	go func(expected string) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		files, err := a.controller.FetchTorrentFiles(ctx, expected)
		fyne.Do(func() {
			if a.activeDetailsHash() != expected || a.detailsState == nil {
				return
			}
			a.detailsState.Content.Loading = false
			a.detailsState.Content.Loaded = err == nil
			if err != nil {
				a.detailsState.Content.Error = err.Error()
			} else {
				a.detailsState.Content.Error = ""
				a.detailsState.Content.Files = files
				if a.detailsState.Content.Expanded == nil {
					a.detailsState.Content.Expanded = map[string]bool{}
				}
			}
			a.refreshDetailsPresentation()
		})
	}(hash)
}

func (a *application) loadTorrentDetailsPeers(hash string, rid int) {
	a.detailsState.Peers.Loading = true
	a.refreshDetailsPresentation()
	go func(expected string, currentRID int) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		data, err := a.controller.FetchTorrentPeers(ctx, expected, currentRID)
		fyne.Do(func() {
			if a.activeDetailsHash() != expected || a.detailsState == nil {
				return
			}
			a.detailsState.Peers.Loading = false
			a.detailsState.Peers.Loaded = err == nil
			if err != nil {
				a.detailsState.Peers.Error = err.Error()
			} else {
				a.detailsState.Peers.Error = ""
				a.detailsState.Peers.RID = data.RID
				a.detailsState.Peers.Peers = sortedPeers(data.Peers)
			}
			a.refreshDetailsPresentation()
		})
	}(hash, rid)
}

func (a *application) loadTorrentDetailsTrackers(hash string) {
	a.detailsState.Trackers.Loading = true
	a.refreshDetailsPresentation()
	go func(expected string) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		data, err := a.controller.FetchTorrentTrackers(ctx, expected)
		fyne.Do(func() {
			if a.activeDetailsHash() != expected || a.detailsState == nil {
				return
			}
			a.detailsState.Trackers.Loading = false
			a.detailsState.Trackers.Loaded = err == nil
			if err != nil {
				a.detailsState.Trackers.Error = err.Error()
			} else {
				a.detailsState.Trackers.Error = ""
				a.detailsState.Trackers.Trackers = data
			}
			a.refreshDetailsPresentation()
		})
	}(hash)
}

func (a *application) loadTorrentDetailsWebSeeds(hash string) {
	a.detailsState.WebSeeds.Loading = true
	a.refreshDetailsPresentation()
	go func(expected string) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		data, err := a.controller.FetchTorrentWebSeeds(ctx, expected)
		fyne.Do(func() {
			if a.activeDetailsHash() != expected || a.detailsState == nil {
				return
			}
			a.detailsState.WebSeeds.Loading = false
			a.detailsState.WebSeeds.Loaded = err == nil
			if err != nil {
				a.detailsState.WebSeeds.Error = err.Error()
			} else {
				a.detailsState.WebSeeds.Error = ""
				a.detailsState.WebSeeds.WebSeeds = data
			}
			a.refreshDetailsPresentation()
		})
	}(hash)
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
