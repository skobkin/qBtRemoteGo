package resources

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

var (
	//go:embed assets/app_light.svg
	appLightSVG []byte
	//go:embed assets/app_dark.svg
	appDarkSVG []byte
)

var (
	appLight = fyne.NewStaticResource("app_light.svg", appLightSVG)
	appDark  = fyne.NewStaticResource("app_dark.svg", appDarkSVG)
)

func AppIcon() fyne.Resource {
	return appDark
}

func TrayIcon() fyne.Resource {
	return appDark
}
