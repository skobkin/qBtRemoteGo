package ui

import (
	"fyne.io/fyne/v2"

	appcore "github.com/skobkin/qbtremotego/internal/app"
)

// installFileDrop opens Add dialogs for .torrent files dropped on the main
// window. Fyne only delivers dropped files, so dropped magnet text is not
// supported; Ctrl+V covers that.
func (a *application) installFileDrop() {
	a.window.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		a.handleDroppedURIs(uris)
	})
}

// handleDroppedURIs opens Add dialogs for dropped .torrent files.
func (a *application) handleDroppedURIs(uris []fyne.URI) {
	batch := droppedInvocation(uris)
	if batch.Empty() {
		a.logger.Debug("drop ignored: no .torrent files", "count", len(uris))
		return
	}
	a.handleInvocation(batch)
}

// droppedInvocation keeps the dropped local files that are existing .torrent
// files.
func droppedInvocation(uris []fyne.URI) appcore.InvocationBatch {
	paths := make([]string, 0, len(uris))
	for _, uri := range uris {
		if uri != nil && uri.Scheme() == "file" {
			paths = append(paths, uri.Path())
		}
	}
	return appcore.ParseInvocationArgs(paths)
}
