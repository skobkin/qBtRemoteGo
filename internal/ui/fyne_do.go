package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

// DeclareFyneDoMigration opts the app into Fyne's single-UI-goroutine model.
// Fyne latches the migration state in a package-level sync.Once
// (internal/build.MigratedToFyneDo) that is first evaluated at the earliest
// fyne.Do call, draw, or app.Run — not at window creation — so this must run
// before any of those. All goroutine-to-UI updates in this package go through
// fyne.Do.
func DeclareFyneDoMigration(fyApp fyne.App) {
	meta := fyApp.Metadata()
	meta.Migrations = map[string]bool{"fyneDo": true}
	app.SetMetadata(meta)
}
