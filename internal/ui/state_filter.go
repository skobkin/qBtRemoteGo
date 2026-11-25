package ui

import (
	"fmt"
	"slices"

	"fyne.io/fyne/v2/widget"

	appcore "github.com/skobkin/qbtremotego/internal/app"
	"github.com/skobkin/qbtremotego/internal/config"
)

// buildStateFilterSelect creates the toolbar quick filter. Its options carry
// live counts, so the chosen bucket is tracked in a.stateFilter rather than
// by option text.
func (a *application) buildStateFilterSelect() *widget.Select {
	a.stateFilter = appcore.ParseStateFilter(a.controller.Config().UI.StateFilter)
	sel := widget.NewSelect(stateFilterOptions(nil), nil)
	sel.SetSelectedIndex(slices.Index(appcore.StateFilters(), a.stateFilter))
	sel.OnChanged = func(string) {
		if a.updatingStateOptions {
			return
		}
		a.setStateFilter(appcore.StateFilters()[sel.SelectedIndex()])
	}
	return sel
}

// setStateFilter switches the quick filter bucket, persists it and refreshes
// the list.
func (a *application) setStateFilter(filter appcore.StateFilter) {
	a.stateFilter = filter
	if err := a.controller.SaveLocalUI(func(cfg *config.AppConfig) {
		cfg.UI.StateFilter = string(filter)
	}); err != nil {
		a.logger.Warn("persist state filter", "filter", filter, "error", err)
	}
	a.refreshVisibleTorrents()
}

// updateStateFilterOptions refreshes the bucket counts without firing the
// select's change handler.
func (a *application) updateStateFilterOptions() {
	if a.stateSelect == nil {
		return
	}
	options := stateFilterOptions(appcore.CountByState(a.allTorrents))
	if slices.Equal(options, a.stateSelect.Options) {
		return
	}
	a.updatingStateOptions = true
	defer func() { a.updatingStateOptions = false }()
	a.stateSelect.SetOptions(options)
	a.stateSelect.SetSelectedIndex(slices.Index(appcore.StateFilters(), a.stateFilter))
}

func stateFilterOptions(counts map[appcore.StateFilter]int) []string {
	filters := appcore.StateFilters()
	options := make([]string, len(filters))
	for i, filter := range filters {
		options[i] = fmt.Sprintf("%s (%d)", appcore.StateFilterLabel(filter), counts[filter])
	}
	return options
}
