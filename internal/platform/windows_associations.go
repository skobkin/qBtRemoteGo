package platform

import (
	"fmt"
	"strings"
)

const (
	windowsAppName                      = "qBtRemoteGo"
	windowsAppRegistrationPath          = `Software\qbtremotego`
	windowsCapabilitiesPath             = windowsAppRegistrationPath + `\Capabilities`
	windowsRegisteredApplicationsPath   = `Software\RegisteredApplications`
	windowsMagnetProgID                 = `qbtremotego.magnet`
	windowsTorrentProgID                = `qbtremotego.torrent`
	windowsMagnetUserChoicePath         = `Software\Microsoft\Windows\Shell\Associations\UrlAssociations\magnet\UserChoice`
	windowsTorrentUserChoicePath        = `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\.torrent\UserChoice`
	windowsMagnetSchemePath             = `Software\Classes\magnet`
	windowsMagnetProgIDPath             = `Software\Classes\` + windowsMagnetProgID
	windowsTorrentExtensionPath         = `Software\Classes\.torrent`
	windowsTorrentProgIDPath            = `Software\Classes\` + windowsTorrentProgID
	windowsDefaultAppsSelectionHintText = "Choose qBtRemoteGo in Settings > Apps > Default apps."
)

func windowsHandlerCommand(exePath string) string {
	return fmt.Sprintf(`"%s" "%%1"`, exePath)
}

func windowsDefaultIcon(exePath string) string {
	return fmt.Sprintf(`"%s",0`, exePath)
}

func isOurWindowsHandlerProgID(expectedProgID string, currentProgID string) bool {
	progID := strings.ToLower(strings.TrimSpace(currentProgID))
	if progID == "" {
		return false
	}

	return progID == strings.ToLower(expectedProgID) || progID == `applications\qbtremotego.exe`
}

func windowsDefaultSelectionWarning(subject string, currentProgID string) string {
	currentProgID = strings.TrimSpace(currentProgID)
	if currentProgID == "" {
		return fmt.Sprintf(
			"Windows registered %s for %s, but it is not the active default. %s",
			windowsAppName,
			subject,
			windowsDefaultAppsSelectionHintText,
		)
	}

	return fmt.Sprintf(
		"Windows registered %s for %s, but the active default is still %q. %s",
		windowsAppName,
		subject,
		currentProgID,
		windowsDefaultAppsSelectionHintText,
	)
}

func WindowsDefaultAppsSelectionHint() string {
	return "On Windows, enabling these options registers qBtRemoteGo as an available handler. If another app is still opening magnet links or .torrent files, choose qBtRemoteGo in Settings > Apps > Default apps."
}
