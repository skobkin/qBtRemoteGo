package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	appcore "github.com/skobkin/qbtremotego/internal/app"
)

// emptyStateKind says which message, if any, replaces an empty torrent list.
type emptyStateKind int

const (
	emptyStateNone emptyStateKind = iota
	emptyStateNotConnected
	emptyStateNoMatches
	emptyStateNoTorrents
)

// emptyStateFor picks the empty-list message. Torrents hidden by filters take
// priority over connection problems (the stale rows are still there); before
// the first refresh completes nothing is shown, so a slow connect doesn't
// flash "No torrents yet".
func emptyStateFor(needsSetup bool, state connectionState, total int, visible int) emptyStateKind {
	switch {
	case visible > 0:
		return emptyStateNone
	case total > 0:
		return emptyStateNoMatches
	case needsSetup || state == connectionStateDisconnected:
		return emptyStateNotConnected
	case state == connectionStateUnknown:
		return emptyStateNone
	default:
		return emptyStateNoTorrents
	}
}

// buildEmptyState creates the centred message and action shown over an empty
// torrent list.
func (a *application) buildEmptyState() fyne.CanvasObject {
	a.emptyStateMessage = widget.NewLabel("")
	a.emptyStateMessage.Alignment = fyne.TextAlignCenter
	a.emptyStateMessage.TextStyle = fyne.TextStyle{Bold: true}
	a.emptyStateAction = widget.NewButton("", nil)
	a.emptyState = container.NewCenter(container.NewVBox(a.emptyStateMessage, container.NewCenter(a.emptyStateAction)))
	a.emptyState.Hide()
	return a.emptyState
}

// syncEmptyState shows, updates or hides the empty-list message.
func (a *application) syncEmptyState() {
	if a.emptyState == nil {
		return
	}
	kind := emptyStateFor(needsConnectionSetup(a.controller.Config()), a.connectionState, len(a.allTorrents), len(a.visibleTorrents))
	switch kind {
	case emptyStateNotConnected:
		a.setEmptyState("Not connected to qBittorrent", "Open settings", a.openSettingsWindow)
	case emptyStateNoMatches:
		a.setEmptyState("No torrents match the current filters", "Clear filters", a.clearFilters)
	case emptyStateNoTorrents:
		a.setEmptyState("No torrents yet", "Add torrent", func() { a.openAddWindow(nil) })
	default:
		a.emptyState.Hide()
	}
}

func (a *application) setEmptyState(message string, action string, onTapped func()) {
	a.emptyStateMessage.SetText(message)
	a.emptyStateAction.SetText(action)
	a.emptyStateAction.OnTapped = onTapped
	a.emptyState.Show()
}

// clearFilters resets the search text and the state filter.
func (a *application) clearFilters() {
	if a.filterEntry != nil {
		a.filterEntry.SetText("")
	}
	a.filterQuery = ""
	a.syncFilterClear("")
	a.setStateFilter(appcore.StateAll)
}
