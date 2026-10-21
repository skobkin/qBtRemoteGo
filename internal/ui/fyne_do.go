package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

// DeclareFyneDoMigration opts the app into Fyne's single-UI-goroutine model
// before the driver performs its once-only migration check; call it right
// after app creation and before the first window. All goroutine-to-UI updates
// in this package go through fyne.Do.
func DeclareFyneDoMigration(fyApp fyne.App) {
	meta := fyApp.Metadata()
	meta.Migrations = map[string]bool{"fyneDo": true}
	app.SetMetadata(meta)
}
