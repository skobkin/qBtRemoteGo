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
	//go:embed assets/status_connection_connected.svg
	statusConnectionConnectedSVG []byte
	//go:embed assets/status_connection_firewalled.svg
	statusConnectionFirewalledSVG []byte
	//go:embed assets/status_connection_disconnected.svg
	statusConnectionDisconnectedSVG []byte
	//go:embed assets/status_slow_mode_on.svg
	statusSlowModeOnSVG []byte
	//go:embed assets/status_slow_mode_off.svg
	statusSlowModeOffSVG []byte
)

var (
	appLight = fyne.NewStaticResource("app_light.svg", appLightSVG)
	appDark  = fyne.NewStaticResource("app_dark.svg", appDarkSVG)

	statusConnectionConnected    = fyne.NewStaticResource("status_connection_connected.svg", statusConnectionConnectedSVG)
	statusConnectionFirewalled   = fyne.NewStaticResource("status_connection_firewalled.svg", statusConnectionFirewalledSVG)
	statusConnectionDisconnected = fyne.NewStaticResource("status_connection_disconnected.svg", statusConnectionDisconnectedSVG)
	statusSlowModeOn             = fyne.NewStaticResource("status_slow_mode_on.svg", statusSlowModeOnSVG)
	statusSlowModeOff            = fyne.NewStaticResource("status_slow_mode_off.svg", statusSlowModeOffSVG)
)

func AppIcon() fyne.Resource {
	return appDark
}

func TrayIcon() fyne.Resource {
	return appDark
}

func ConnectionStatusIcon(status string) fyne.Resource {
	switch status {
	case "connected":
		return statusConnectionConnected
	case "firewalled":
		return statusConnectionFirewalled
	default:
		return statusConnectionDisconnected
	}
}

func SlowModeIcon(enabled bool) fyne.Resource {
	if enabled {
		return statusSlowModeOn
	}
	return statusSlowModeOff
}
