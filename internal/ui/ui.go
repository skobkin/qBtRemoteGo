package ui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"fyne.io/systray"

	appcore "github.com/skobkin/qbtremotego/internal/app"
	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/credentials"
	"github.com/skobkin/qbtremotego/internal/logging"
	"github.com/skobkin/qbtremotego/internal/platform"
	"github.com/skobkin/qbtremotego/internal/qbt"
	"github.com/skobkin/qbtremotego/internal/resources"
)

type application struct {
	fyApp  fyne.App
	window fyne.Window
	logger *slog.Logger

	controller *appcore.Controller
	logManager *logging.Manager

	allTorrents       []qbt.Torrent
	visibleTorrents   []qbt.Torrent
	selection         map[string]bool
	selectionAnchor   string
	filterQuery       string
	transfer          qbt.TransferInfo
	serverState       qbt.ServerState
	serverStateKnown  bool
	lastError         string
	windowVisible     bool
	trayAvailable     bool
	settingsWindow    fyne.Window
	pendingInvocation appcore.InvocationBatch

	list           *widget.List
	tableHeader    *torrentHeaderRow
	tablePreview   *canvas.Rectangle
	tableScroll    *container.Scroll
	statusLabel    *widget.Label
	connectionIcon *hoverIcon
	slowModeIcon   *hoverIcon
	filterEntry    *widget.Entry
	filterBy       *widget.Select
	tooltipLayer   *tooltipOverlay
	tooltipManager *hoverTooltipManager
	columnWidths   map[string]float32
	previewX       float32

	trayState trayState

	filterTimer *time.Timer
}

type trayState struct {
	desktopApp desktop.App
	showItem   *fyne.MenuItem
	speedItem  *fyne.MenuItem
	quitItem   *fyne.MenuItem
}

func Run(initialInvocation appcore.InvocationBatch, activations <-chan appcore.InvocationBatch) error {
	configPath, err := config.DefaultConfigPath()
	if err != nil {
		return err
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	paths, err := appcore.ResolvePaths()
	if err != nil {
		return err
	}

	logManager, err := logging.New(cfg.Logging)
	if err != nil {
		return err
	}
	// Open the log file (if enabled) so the bootstrap message below
	// also lands on disk. If the file cannot be opened, fall back to
	// stdout-only rather than aborting startup.
	if cfgErr := logManager.Configure(cfg.Logging, paths.LogFile); cfgErr != nil {
		slog.Warn("open log file at startup; continuing with stdout only", "error", cfgErr, "path", paths.LogFile)
		if _, lerr := logging.New(cfg.Logging); lerr != nil {
			return fmt.Errorf("configure logger: %w (fallback: %w)", cfgErr, lerr)
		}
	}

	logManager.Logger("bootstrap").Info(
		"starting qbtremotego",
		"version", appcore.BuildVersion(),
		"build_date", appcore.BuildDateYMD(),
	)

	controller, err := appcore.NewController(configPath, logManager.Logger("controller"))
	if err != nil {
		return err
	}

	fyApp := app.NewWithID(appcore.ID)
	fyApp.SetIcon(resources.AppIcon())

	window := fyApp.NewWindow(appcore.Name)
	window.Resize(fyne.NewSize(1120, 680))
	window.SetIcon(resources.AppIcon())

	ui := &application{
		fyApp:         fyApp,
		window:        window,
		logger:        logManager.Logger("ui"),
		controller:    controller,
		logManager:    logManager,
		selection:     map[string]bool{},
		windowVisible: true,
		statusLabel:   widget.NewLabel(""),
	}

	ui.buildMainWindow()
	ui.configureTray()
	ui.bindCloseBehavior()

	startupSyncWarnings := platform.JoinErrors(controller.SyncIntegrations())
	setupNeeded := needsConnectionSetup(controller.Config())

	startHidden := !setupNeeded && controller.Config().UI.StartMinimizedToTray && ui.trayAvailable && initialInvocation.Empty() && startupSyncWarnings == ""
	if startHidden {
		ui.windowVisible = false
	} else {
		window.Show()
	}

	if startupSyncWarnings != "" {
		dialog.ShowError(fmt.Errorf("integration warnings:\n%s", startupSyncWarnings), window)
	}

	if setupNeeded {
		fyne.Do(func() {
			ui.deferInvocationUntilConnectionSetup(initialInvocation)
			ui.openSettingsWindow()
		})
	} else if !initialInvocation.Empty() {
		fyne.Do(func() {
			ui.handleInvocation(initialInvocation)
		})
	}

	go func() {
		for batch := range activations {
			if batch.Empty() {
				continue
			}
			batch := batch
			fyne.Do(func() {
				ui.handleOrDeferInvocation(batch)
			})
		}
	}()

	go ui.pollLoop()
	fyApp.Run()
	_ = logManager.Close()

	return nil
}

func (a *application) buildMainWindow() {
	a.tooltipLayer = newTooltipOverlay()
	a.tooltipManager = newHoverTooltipManager(a.tooltipLayer)

	addButton := newButtonWithTooltip(a.tooltipManager, theme.ContentAddIcon(), "Add torrent", func() {
		a.openAddWindow(nil)
	})
	removeButton := newButtonWithTooltip(a.tooltipManager, theme.DeleteIcon(), "Remove selected torrents", func() {
		a.confirmDelete()
	})
	startButton := newButtonWithTooltip(a.tooltipManager, theme.MediaPlayIcon(), "Start selected torrents", func() {
		a.startSelectedTorrents()
	})
	stopButton := newButtonWithTooltip(a.tooltipManager, theme.MediaStopIcon(), "Stop selected torrents", func() {
		a.stopSelectedTorrents()
	})
	settingsButton := newButtonWithTooltip(a.tooltipManager, theme.SettingsIcon(), "Settings", func() {
		a.openSettingsWindow()
	})

	a.filterEntry = widget.NewEntry()
	a.filterEntry.SetPlaceHolder("Filter torrents")
	a.filterEntry.OnChanged = func(value string) {
		if a.filterTimer != nil {
			a.filterTimer.Stop()
		}
		a.filterTimer = time.AfterFunc(200*time.Millisecond, func() {
			fyne.Do(func() {
				a.filterQuery = value
				a.refreshVisibleTorrents()
			})
		})
	}

	a.filterBy = widget.NewSelect([]string{"Name", "Location"}, func(value string) {
		cfg := a.controller.Config()
		cfg.UI.FilterBy = strings.ToLower(value)
		if err := a.controller.SaveLocalUI(cfg); err != nil {
			a.logger.Warn("persist filter_by", "error", err)
		}
		a.refreshVisibleTorrents()
	})
	if strings.EqualFold(a.controller.Config().UI.FilterBy, "location") {
		a.filterBy.SetSelected("Location")
	} else {
		a.filterBy.SetSelected("Name")
	}

	filterLabel := widget.NewLabel("Filter by")
	filterSelect := container.NewGridWrap(fyne.NewSize(140, a.filterBy.MinSize().Height), a.filterBy)
	filterInput := container.NewGridWrap(fyne.NewSize(240, a.filterEntry.MinSize().Height), a.filterEntry)
	leftTools := container.NewHBox(
		addButton,
		removeButton,
		startButton,
		stopButton,
		settingsButton,
	)
	rightTools := container.NewHBox(
		filterLabel,
		filterSelect,
		filterInput,
	)

	a.statusLabel.Truncation = fyne.TextTruncateEllipsis
	a.connectionIcon = newHoverIcon(a.tooltipManager, resources.ConnectionStatusIcon(""), "Connection status unavailable")
	a.slowModeIcon = newHoverIcon(a.tooltipManager, resources.SlowModeIcon(false), "Slow mode: off")

	toolbar := container.NewBorder(nil, nil, leftTools, rightTools)

	center := a.buildTorrentTable()
	statusIcons := container.NewHBox(a.slowModeIcon, a.connectionIcon)
	statusBar := container.NewBorder(nil, nil, nil, statusIcons, a.statusLabel)
	bottom := container.NewBorder(widget.NewSeparator(), nil, nil, nil, container.NewPadded(statusBar))
	content := container.NewBorder(toolbar, bottom, nil, nil, center)
	a.window.SetContent(container.NewStack(content, a.tooltipLayer))

	a.refreshVisibleTorrents()
}

func (a *application) bindCloseBehavior() {
	if !a.trayAvailable {
		return
	}
	a.window.SetCloseIntercept(func() {
		a.windowVisible = false
		a.window.Hide()
	})
}

func (a *application) configureTray() {
	desk, ok := a.fyApp.(desktop.App)
	if !ok {
		return
	}

	a.trayAvailable = true
	a.trayState = trayState{
		desktopApp: desk,
		speedItem:  fyne.NewMenuItem("Down 0 B/s | Up 0 B/s", nil),
		showItem: fyne.NewMenuItem("Open main window", func() {
			fyne.Do(func() {
				a.windowVisible = true
				a.window.Show()
				a.window.RequestFocus()
			})
		}),
		quitItem: fyne.NewMenuItem("Quit application", func() {
			a.fyApp.Quit()
		}),
	}
	a.trayState.speedItem.Disabled = true
	desk.SetSystemTrayIcon(resources.TrayIcon())
	desk.SetSystemTrayMenu(fyne.NewMenu(appcore.Name, a.trayState.speedItem, a.trayState.showItem, a.trayState.quitItem))
	systray.SetOnTapped(func() {
		fyne.Do(func() {
			a.windowVisible = true
			a.window.Show()
			a.window.RequestFocus()
		})
	})
	systray.SetTooltip("Down 0 B/s | Up 0 B/s")
}

func (a *application) openSettingsWindow() {
	if a.settingsWindow != nil {
		a.settingsWindow.Show()
		a.settingsWindow.RequestFocus()
		return
	}

	cfg := a.controller.Config()

	win := a.fyApp.NewWindow("Settings")
	a.settingsWindow = win
	win.Resize(fyne.NewSize(620, 560))
	win.SetOnClosed(func() {
		if a.settingsWindow == win {
			a.settingsWindow = nil
		}
	})

	urlEntry := widget.NewEntry()
	urlEntry.SetText(cfg.Connection.URL)
	usernameEntry := widget.NewEntry()
	usernameEntry.SetText(a.controller.SessionCredentials().Username)
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetText(a.controller.SessionCredentials().Password)
	skipTLS := widget.NewCheck("", nil)
	skipTLS.SetChecked(cfg.Connection.SkipCertificateCheck)
	testStatus := widget.NewLabel("")
	credentialSummary := widget.NewLabel(connectionCredentialStorageText(
		cfg.Connection.CredentialStorage,
		a.controller.CredentialStatus(),
		a.controller.SessionCredentials(),
	))
	credentialWarning := widget.NewLabel(connectionCredentialWarningText(
		cfg.Connection.CredentialStorage,
		a.controller.CredentialStatus(),
		a.controller.SessionCredentials(),
	))
	credentialWarning.Wrapping = fyne.TextWrapWord

	rememberEntry := widget.NewEntry()
	rememberEntry.SetText(fmt.Sprintf("%d", cfg.UI.RememberPathCount))
	rememberEntry.Validator = numberValidator
	rememberLastSaveLocation := widget.NewCheck("", nil)
	rememberLastSaveLocation.SetChecked(cfg.UI.RememberLastSaveLocation)
	autocomplete := widget.NewCheck("", nil)
	autocomplete.SetChecked(cfg.UI.PathAutocomplete)
	activePoll := widget.NewEntry()
	activePoll.SetText(fmt.Sprintf("%d", cfg.UI.ActivePollSeconds))
	activePoll.Validator = numberValidator
	backgroundPoll := widget.NewEntry()
	backgroundPoll.SetText(fmt.Sprintf("%d", cfg.UI.BackgroundPollSeconds))
	backgroundPoll.Validator = numberValidator
	startMinimized := widget.NewCheck("", nil)
	startMinimized.SetChecked(cfg.UI.StartMinimizedToTray)
	sortBy := widget.NewSelect(sortColumnLabels(), nil)
	sortBy.SetSelected(sortColumnLabel(cfg.UI.SortColumn))
	sortDescending := widget.NewCheck("", nil)
	sortDescending.SetChecked(cfg.UI.SortDescending)
	logLevel := widget.NewSelect(logging.SupportedLevels(), nil)
	logLevel.SetSelected(cfg.Logging.Level)
	logToFile := widget.NewCheck("", nil)
	logToFile.SetChecked(cfg.Logging.LogToFile)

	registerMagnet := widget.NewCheck("", nil)
	registerMagnet.SetChecked(cfg.Integration.RegisterMagnetHandler)
	registerTorrent := widget.NewCheck("", nil)
	registerTorrent.SetChecked(cfg.Integration.RegisterTorrentHandler)
	startWithSystem := widget.NewCheck("", nil)
	startWithSystem.SetChecked(cfg.Integration.StartWithSystem)

	connectionForm := widget.NewForm(
		widget.NewFormItem("URL", urlEntry),
		widget.NewFormItem("Username", usernameEntry),
		widget.NewFormItem("Password", passwordEntry),
		widget.NewFormItem("Skip certificate validation", skipTLS),
		widget.NewFormItem("Credential storage", credentialSummary),
	)

	uiForm := widget.NewForm(
		widget.NewFormItem("Number of paths to remember", rememberEntry),
		widget.NewFormItem("Remember last save location", rememberLastSaveLocation),
		widget.NewFormItem("Path autocompletion", autocomplete),
		widget.NewFormItem("Torrent list update time (seconds)", activePoll),
		widget.NewFormItem("Background update time (seconds)", backgroundPoll),
		widget.NewFormItem("Start minimized to tray", startMinimized),
		widget.NewFormItem("Sort by", sortBy),
		widget.NewFormItem("Descending order", sortDescending),
		widget.NewFormItem("Log level", logLevel),
		widget.NewFormItem("Log to file", logToFile),
	)

	integrationForm := widget.NewForm(
		widget.NewFormItem("Register magnet handler", registerMagnet),
		widget.NewFormItem("Register .torrent handler", registerTorrent),
		widget.NewFormItem("Start with the system", startWithSystem),
	)
	integrationContent := []fyne.CanvasObject{integrationForm}
	if runtime.GOOS == "windows" {
		integrationHint := widget.NewLabel(platform.WindowsDefaultAppsSelectionHint())
		integrationHint.Wrapping = fyne.TextWrapWord
		integrationContent = append(integrationContent, integrationHint)
	}

	testButton := widget.NewButton("Test connection", func() {
		testStatus.SetText("Testing connection...")
		go func() {
			err := a.controller.TestConnection(context.Background(), config.ConnectionConfig{
				URL:                  urlEntry.Text,
				SkipCertificateCheck: skipTLS.Checked,
			}, credentials.Credentials{
				Username: usernameEntry.Text,
				Password: passwordEntry.Text,
			})
			fyne.Do(func() {
				if err != nil {
					testStatus.SetText("Connection test failed: " + err.Error())
					return
				}
				testStatus.SetText("Connection successful.")
			})
		}()
	})

	finishSave := func(updated config.AppConfig) {
		if cfg.Logging.Level != updated.Logging.Level || cfg.Logging.LogToFile != updated.Logging.LogToFile {
			paths, pathsErr := appcore.ResolvePaths()
			if pathsErr != nil {
				dialog.ShowError(fmt.Errorf("resolve app paths: %w", pathsErr), win)
				return
			}
			if err := a.logManager.Configure(updated.Logging, paths.LogFile); err != nil {
				dialog.ShowError(fmt.Errorf("apply log settings: %w", err), win)
				return
			}
			a.logger = a.logManager.Logger("ui")
			a.controller.SetLogger(a.logManager.Logger("controller"))
			a.logger.Info(
				"updated log settings from settings",
				"level", updated.Logging.Level,
				"log_to_file", updated.Logging.LogToFile,
			)
		}
		a.refreshVisibleTorrents()
		win.Close()
		a.handlePendingInvocationAfterConnectionSetup()
	}

	saveButton := widget.NewButton("Save", func() {
		rememberCount, err := parsePositiveInt(rememberEntry.Text)
		if err != nil {
			dialog.ShowError(fmt.Errorf("number of remembered paths: %w", err), win)
			return
		}
		activeSeconds, err := parsePositiveInt(activePoll.Text)
		if err != nil {
			dialog.ShowError(fmt.Errorf("active update time: %w", err), win)
			return
		}
		backgroundSeconds, err := parsePositiveInt(backgroundPoll.Text)
		if err != nil {
			dialog.ShowError(fmt.Errorf("background update time: %w", err), win)
			return
		}

		updated := a.controller.Config()
		updated.Connection.URL = urlEntry.Text
		updated.Connection.SkipCertificateCheck = skipTLS.Checked
		updated.UI.RememberPathCount = rememberCount
		updated.UI.RememberLastSaveLocation = rememberLastSaveLocation.Checked
		updated.UI.PathAutocomplete = autocomplete.Checked
		updated.UI.ActivePollSeconds = activeSeconds
		updated.UI.BackgroundPollSeconds = backgroundSeconds
		updated.UI.StartMinimizedToTray = startMinimized.Checked
		updated.UI.SortColumn = sortColumnKey(sortBy.Selected)
		updated.UI.SortDescending = sortDescending.Checked
		updated.Logging.Level = logLevel.Selected
		updated.Logging.LogToFile = logToFile.Checked
		updated.Integration.RegisterMagnetHandler = registerMagnet.Checked
		updated.Integration.RegisterTorrentHandler = registerTorrent.Checked
		updated.Integration.StartWithSystem = startWithSystem.Checked

		editedCreds := credentials.Credentials{
			Username: usernameEntry.Text,
			Password: passwordEntry.Text,
		}

		var saveWithFallback func(appcore.CredentialFallbackChoice)
		saveWithFallback = func(fallback appcore.CredentialFallbackChoice) {
			result, err := a.controller.SaveSettings(context.Background(), updated, editedCreds, fallback)
			if result.DecisionRequired {
				message := "System keychain is unavailable.\n\nPress OK to save the connection credentials in plain text in the local config file.\nPress Cancel to keep the edited credentials only for this session."
				if result.CredentialStatus.Message != "" {
					message = result.CredentialStatus.Message + "\n\nPress OK to save the connection credentials in plain text in the local config file.\nPress Cancel to keep the edited credentials only for this session."
				}
				dialog.ShowConfirm("Store credentials as plain text?", message, func(ok bool) {
					if ok {
						saveWithFallback(appcore.CredentialFallbackPlaintext)
						return
					}
					saveWithFallback(appcore.CredentialFallbackSessionOnly)
				}, win)

				return
			}
			if err != nil {
				dialog.ShowError(fmt.Errorf("settings saved with integration warnings:\n%w", err), win)
			}
			finishSave(updated)
		}

		saveWithFallback(appcore.CredentialFallbackUnspecified)
	})

	tabs := container.NewAppTabs(
		container.NewTabItem("Connection", container.NewPadded(container.NewVBox(
			connectionForm,
			credentialWarning,
			testButton,
			testStatus,
		))),
		container.NewTabItem("UI", container.NewPadded(uiForm)),
		container.NewTabItem("Integration", container.NewPadded(container.NewVBox(integrationContent...))),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	content := container.NewBorder(
		nil,
		container.NewPadded(container.NewHBox(layout.NewSpacer(), saveButton)),
		nil,
		nil,
		tabs,
	)

	win.SetContent(content)
	win.Show()
}

func needsConnectionSetup(cfg config.AppConfig) bool {
	return strings.TrimSpace(cfg.Connection.URL) == ""
}

func mergeInvocationBatches(dst, src appcore.InvocationBatch) appcore.InvocationBatch {
	dst.MagnetLinks = append(dst.MagnetLinks, src.MagnetLinks...)
	dst.TorrentFiles = append(dst.TorrentFiles, src.TorrentFiles...)

	return dst
}

func (a *application) handleOrDeferInvocation(batch appcore.InvocationBatch) {
	if needsConnectionSetup(a.controller.Config()) {
		a.deferInvocationUntilConnectionSetup(batch)
		a.windowVisible = true
		a.window.Show()
		a.window.RequestFocus()
		a.openSettingsWindow()
		return
	}

	a.handleInvocation(batch)
}

func (a *application) deferInvocationUntilConnectionSetup(batch appcore.InvocationBatch) {
	if batch.Empty() {
		return
	}
	a.pendingInvocation = mergeInvocationBatches(a.pendingInvocation, batch)
}

func (a *application) handlePendingInvocationAfterConnectionSetup() {
	if needsConnectionSetup(a.controller.Config()) || a.pendingInvocation.Empty() {
		return
	}

	pending := a.pendingInvocation
	a.pendingInvocation = appcore.InvocationBatch{}
	a.handleInvocation(pending)
}

func connectionCredentialStorageText(
	mode config.CredentialStorageMode,
	status credentials.Status,
	session credentials.Credentials,
) string {
	switch mode {
	case config.CredentialStorageKeychain:
		if status.Backend != "" {
			return "System keychain (" + status.Backend + ")"
		}
		return "System keychain"
	case config.CredentialStoragePlaintext:
		return "Plain text config file"
	default:
		if strings.TrimSpace(session.Username) != "" || session.Password != "" {
			return "Session only"
		}
		return "Not saved"
	}
}

func connectionCredentialWarningText(
	mode config.CredentialStorageMode,
	status credentials.Status,
	session credentials.Credentials,
) string {
	switch mode {
	case config.CredentialStorageKeychain:
		if status.State == credentials.StateAvailable {
			return ""
		}
		message := status.Message
		if strings.TrimSpace(message) == "" {
			message = "System keychain is currently unavailable."
		}
		return "Warning: Credentials are configured to use the system keychain, but it is currently unavailable. " + message
	case config.CredentialStoragePlaintext:
		return "Warning: Credentials are stored in plain text in the local config file."
	default:
		if strings.TrimSpace(session.Username) != "" || session.Password != "" {
			return "Credentials are stored only in memory for this run."
		}
		return ""
	}
}

func (a *application) handleInvocation(batch appcore.InvocationBatch) {
	a.logger.Info("handling activation in UI", "magnet_links", batch.MagnetLinks, "torrent_files", batch.TorrentFiles)
	a.windowVisible = true
	a.window.Show()
	a.window.RequestFocus()

	if len(batch.MagnetLinks) > 0 {
		a.openAddWindow(&appcore.AddDialogPrefill{
			SourceType:  qbt.SourceMagnet,
			MagnetLinks: batch.MagnetLinks,
		})
	}
	for _, torrentFile := range batch.TorrentFiles {
		a.openAddWindow(&appcore.AddDialogPrefill{
			SourceType:      qbt.SourceTorrentFile,
			TorrentFilePath: torrentFile,
		})
	}
}

func (a *application) openAddWindow(prefill *appcore.AddDialogPrefill) {
	win := a.fyApp.NewWindow("Add Torrent")

	cfg := a.controller.Config()
	win.Resize(addTorrentWindowSize(cfg.UI.AddTorrentAdvancedExpanded))
	data := appcore.AddDialogData{
		SourceType:     qbt.SourceMagnet,
		ManagementMode: "Manual",
		StartTorrent:   true,
		StopCondition:  "None",
		ContentLayout:  "Original",
		SavePath:       "",
	}
	if prefill != nil {
		data.SourceType = prefill.SourceType
		data.TorrentFilePath = prefill.TorrentFilePath
		data.MagnetText = strings.Join(prefill.MagnetLinks, "\n")
	}
	var shouldFetchDefaultSavePath bool
	data.SavePath, shouldFetchDefaultSavePath = initialAddDialogSavePath(cfg.UI)

	categories, tags, preloadErr := a.controller.FetchCategoriesAndTags(context.Background())
	if preloadErr != nil {
		a.logger.Info("preload categories/tags", "error", preloadErr)
	}

	status := widget.NewLabel("")
	status.Hide()
	setStatus := func(message string) {
		status.SetText(message)
		if strings.TrimSpace(message) == "" {
			status.Hide()
			return
		}
		status.Show()
	}

	sourceSelect := widget.NewRadioGroup([]string{"Torrent file", "Magnet links"}, nil)
	if data.SourceType == qbt.SourceTorrentFile {
		sourceSelect.SetSelected("Torrent file")
	} else {
		sourceSelect.SetSelected("Magnet links")
	}

	fileEntry := widget.NewEntry()
	fileEntry.SetText(data.TorrentFilePath)
	magnetEntry := widget.NewMultiLineEntry()
	magnetEntry.SetMinRowsVisible(4)
	magnetEntry.SetText(data.MagnetText)
	sourceContainer := container.NewStack()

	savePathEntry := newPathAutocompleteEntry(
		cfg.UI.RecentSavePaths,
		a.controller.SuggestDirectories,
		setStatus,
	)
	savePathEntry.SetText(data.SavePath)
	categoryEntry := widget.NewSelectEntry(categories)
	tagsEntry := widget.NewEntry()
	if len(tags) > 0 {
		tagsEntry.SetPlaceHolder("Known tags: " + strings.Join(tags, ", "))
	}
	managementSelect := widget.NewSelect([]string{"Manual", "Auto"}, nil)
	managementSelect.SetSelected(data.ManagementMode)
	stopSelect := widget.NewSelect([]string{"None", "Metadata received", "Files checked"}, nil)
	stopSelect.SetSelected(data.StopCondition)
	contentLayoutSelect := widget.NewSelect([]string{"Original", "Create subfolder", "Don't create subfolder"}, nil)
	contentLayoutSelect.SetSelected(data.ContentLayout)

	renameEntry := widget.NewEntry()
	startCheck := widget.NewCheck("", nil)
	startCheck.SetChecked(true)
	topOfQueue := widget.NewCheck("", nil)
	skipHashCheck := widget.NewCheck("", nil)
	sequential := widget.NewCheck("", nil)
	firstLastPieces := widget.NewCheck("", nil)

	downloadLimitEntry := widget.NewEntry()
	downloadLimitEntry.Validator = optionalNumberValidator
	uploadLimitEntry := widget.NewEntry()
	uploadLimitEntry.Validator = optionalNumberValidator

	browseButton := widget.NewButtonWithIcon("Browse", theme.FolderOpenIcon(), func() {
		fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				setStatus(err.Error())
				return
			}
			if reader == nil {
				return
			}
			fileEntry.SetText(reader.URI().Path())
			_ = reader.Close()
		}, win)
		fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".torrent"}))
		fileDialog.Show()
	})

	updateSource := func(selected string) {
		if selected == "Torrent file" {
			sourceContainer.Objects = []fyne.CanvasObject{
				container.NewBorder(nil, nil, nil, browseButton, fileEntry),
			}
			data.SourceType = qbt.SourceTorrentFile
		} else {
			sourceContainer.Objects = []fyne.CanvasObject{magnetEntry}
			data.SourceType = qbt.SourceMagnet
		}
		sourceContainer.Refresh()
	}
	sourceSelect.OnChanged = updateSource
	updateSource(sourceSelect.Selected)

	basicItems, advancedItems := buildAddTorrentFormSections(addTorrentFormControls{
		sourceSelect:       sourceSelect,
		sourceContainer:    sourceContainer,
		savePathEntry:      savePathEntry,
		categoryEntry:      categoryEntry,
		startCheck:         startCheck,
		managementSelect:   managementSelect,
		renameEntry:        renameEntry,
		tagsEntry:          tagsEntry,
		topOfQueue:         topOfQueue,
		stopSelect:         stopSelect,
		skipHashCheck:      skipHashCheck,
		contentLayout:      contentLayoutSelect,
		sequential:         sequential,
		firstLastPieces:    firstLastPieces,
		downloadLimitEntry: downloadLimitEntry,
		uploadLimitEntry:   uploadLimitEntry,
	})
	basicForm := widget.NewForm(basicItems...)
	advancedForm := widget.NewForm(advancedItems...)
	advancedAccordion, advancedItem := newAddTorrentAdvancedAccordion(advancedForm, cfg.UI.AddTorrentAdvancedExpanded)
	formContent := container.NewVBox(
		basicForm,
		advancedAccordion,
	)

	var submit *widget.Button
	submit = widget.NewButton("Add", func() {
		data.TorrentFilePath = fileEntry.Text
		data.MagnetText = magnetEntry.Text
		data.ManagementMode = managementSelect.Selected
		data.SavePath = savePathEntry.Text
		data.Rename = renameEntry.Text
		data.Category = categoryEntry.Text
		data.Tags = tagsEntry.Text
		data.StartTorrent = startCheck.Checked
		data.TopOfQueue = topOfQueue.Checked
		data.StopCondition = stopSelect.Selected
		data.SkipHashCheck = skipHashCheck.Checked
		data.ContentLayout = contentLayoutSelect.Selected
		data.SequentialDownload = sequential.Checked
		data.FirstLastPieces = firstLastPieces.Checked
		data.DownloadLimitText = downloadLimitEntry.Text
		data.UploadLimitText = uploadLimitEntry.Text

		setStatus("Submitting torrent...")
		submit.Disable()
		go func() {
			err := a.controller.AddTorrent(context.Background(), data)
			fyne.Do(func() {
				submit.Enable()
				if err != nil {
					setStatus("Add torrent failed: " + err.Error())
					return
				}
				setStatus("Torrent submitted.")
				win.Close()
				a.refreshNow()
			})
		}()
	})

	cancel := widget.NewButton("Cancel", func() {
		win.Close()
	})

	content := container.NewBorder(
		nil,
		container.NewVBox(status, container.NewHBox(layout.NewSpacer(), submit, cancel)),
		nil,
		nil,
		container.NewVScroll(formContent),
	)

	defaultSavePathCtx, cancelDefaultSavePath := context.WithCancel(context.Background())
	var addWindowClosed atomic.Bool

	win.SetContent(content)
	win.SetOnClosed(func() {
		addWindowClosed.Store(true)
		cancelDefaultSavePath()
		savePathEntry.Close()

		current := a.controller.Config()
		if current.UI.AddTorrentAdvancedExpanded == advancedItem.Open {
			return
		}
		current.UI.AddTorrentAdvancedExpanded = advancedItem.Open
		if err := a.controller.SaveLocalUI(current); err != nil {
			a.logger.Warn("save add torrent advanced state", "error", err)
		}
	})
	win.Show()

	if shouldFetchDefaultSavePath {
		go func() {
			defaultSavePath, err := a.controller.FetchDefaultSavePath(defaultSavePathCtx)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					a.logger.Info("load default save path", "error", err)
				}
				return
			}

			fyne.Do(func() {
				if addWindowClosed.Load() || !shouldApplyLazySavePath(savePathEntry.Text, defaultSavePath) {
					return
				}
				savePathEntry.SetText(strings.TrimSpace(defaultSavePath))
			})
		}()
	}
}

func initialAddDialogSavePath(cfg config.UIConfig) (string, bool) {
	if cfg.RememberLastSaveLocation && len(cfg.RecentSavePaths) > 0 {
		return cfg.RecentSavePaths[0], false
	}

	return "", true
}

func shouldApplyLazySavePath(current string, fetched string) bool {
	return strings.TrimSpace(current) == "" && strings.TrimSpace(fetched) != ""
}

type addTorrentFormControls struct {
	sourceSelect       fyne.CanvasObject
	sourceContainer    fyne.CanvasObject
	savePathEntry      fyne.CanvasObject
	categoryEntry      fyne.CanvasObject
	startCheck         fyne.CanvasObject
	managementSelect   fyne.CanvasObject
	renameEntry        fyne.CanvasObject
	tagsEntry          fyne.CanvasObject
	topOfQueue         fyne.CanvasObject
	stopSelect         fyne.CanvasObject
	skipHashCheck      fyne.CanvasObject
	contentLayout      fyne.CanvasObject
	sequential         fyne.CanvasObject
	firstLastPieces    fyne.CanvasObject
	downloadLimitEntry fyne.CanvasObject
	uploadLimitEntry   fyne.CanvasObject
}

func buildAddTorrentFormSections(controls addTorrentFormControls) (basic []*widget.FormItem, advanced []*widget.FormItem) {
	basic = []*widget.FormItem{
		widget.NewFormItem("Source type", controls.sourceSelect),
		widget.NewFormItem("Source", controls.sourceContainer),
		widget.NewFormItem("Save location", controls.savePathEntry),
		widget.NewFormItem("Category", controls.categoryEntry),
		widget.NewFormItem("Start torrent", controls.startCheck),
	}
	advanced = []*widget.FormItem{
		widget.NewFormItem("Torrent management mode", controls.managementSelect),
		widget.NewFormItem("Name override", controls.renameEntry),
		widget.NewFormItem("Tags", controls.tagsEntry),
		widget.NewFormItem("Top of queue", controls.topOfQueue),
		widget.NewFormItem("Stop condition", controls.stopSelect),
		widget.NewFormItem("Skip hash check", controls.skipHashCheck),
		widget.NewFormItem("Content layout", controls.contentLayout),
		widget.NewFormItem("Download sequentially", controls.sequential),
		widget.NewFormItem("Download first and last pieces first", controls.firstLastPieces),
		widget.NewFormItem("Limit download rate (KiB/s)", controls.downloadLimitEntry),
		widget.NewFormItem("Limit upload rate (KiB/s)", controls.uploadLimitEntry),
	}

	return basic, advanced
}

func newAddTorrentAdvancedAccordion(detail fyne.CanvasObject, open bool) (*widget.Accordion, *widget.AccordionItem) {
	item := widget.NewAccordionItem("Advanced", detail)
	item.Open = open
	return widget.NewAccordion(item), item
}

func addTorrentWindowSize(advancedExpanded bool) fyne.Size {
	if advancedExpanded {
		return fyne.NewSize(720, 680)
	}
	return fyne.NewSize(720, 480)
}

func (a *application) refreshVisibleTorrents() {
	cfg := a.controller.Config()
	a.visibleTorrents = appcore.FilterAndSortTorrents(
		a.allTorrents,
		a.filterQuery,
		strings.ToLower(a.filterBy.Selected),
		cfg.UI.SortColumn,
		cfg.UI.SortDescending,
	)
	a.pruneSelectionToVisible()
	if a.list != nil {
		a.list.Refresh()
	}
	a.statusLabel.SetText(a.statusText())
	a.refreshStatusIcons()
}

func (a *application) statusText() string {
	parts := []string{
		fmt.Sprintf("📦 %d", len(a.allTorrents)),
		fmt.Sprintf("🔎 %d", len(a.visibleTorrents)),
		fmt.Sprintf("⬇️ %s", appcore.HumanSpeed(a.transfer.DownloadSpeed)),
		fmt.Sprintf("⬆️ %s", appcore.HumanSpeed(a.transfer.UploadSpeed)),
		fmt.Sprintf("Lim ⬇️:%s ⬆️:%s", appcore.HumanSpeedLimit(a.transfer.DownloadLimit), appcore.HumanSpeedLimit(a.transfer.UploadLimit)),
	}
	if a.serverStateKnown {
		parts = append(parts, "Free "+appcore.HumanBytes(a.serverState.FreeSpaceOnDisk))
	}
	if strings.TrimSpace(a.lastError) != "" {
		parts = append(parts, "Last error: "+a.lastError)
	}
	return strings.Join(parts, " | ")
}

func (a *application) refreshStatusIcons() {
	if a.connectionIcon != nil {
		status := strings.ToLower(strings.TrimSpace(a.transfer.ConnectionStatus))
		tooltip := "Connection status: " + appcore.ConnectionStatusLabel(status)
		switch status {
		case "connected":
			tooltip += ". Incoming connections are available."
		case "firewalled":
			tooltip += ". Incoming connections appear blocked."
		case "disconnected":
			tooltip += ". qBittorrent reports no peer connectivity."
		default:
			tooltip = "Connection status unavailable"
		}
		a.connectionIcon.SetState(resources.ConnectionStatusIcon(status), tooltip)
	}
	if a.slowModeIcon != nil {
		tooltip := "Slow mode: off. Alternative speed limits are disabled."
		if a.serverState.UseAltSpeedLimits {
			tooltip = "Slow mode: on. Alternative speed limits are enabled."
		}
		a.slowModeIcon.SetState(resources.SlowModeIcon(a.serverState.UseAltSpeedLimits), tooltip)
	}
}

func (a *application) pollLoop() {
	a.refreshNow()
	for {
		cfg := a.controller.Config()
		interval := time.Duration(cfg.UI.ActivePollSeconds) * time.Second
		if !a.windowVisible {
			interval = time.Duration(cfg.UI.BackgroundPollSeconds) * time.Second
		}
		time.Sleep(interval)
		a.refreshNow()
	}
}

func (a *application) refreshNow() {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	torrents, err := a.controller.FetchTorrents(ctx)
	transfer, transferErr := a.controller.FetchTransferInfo(ctx)
	serverState, serverStateErr := a.controller.FetchServerState(ctx)

	fyne.Do(func() {
		if err == nil {
			a.allTorrents = torrents
			a.lastError = ""
		} else {
			a.lastError = err.Error()
		}
		if transferErr == nil {
			a.transfer = transfer
		} else if a.lastError == "" {
			a.lastError = transferErr.Error()
		}
		if serverStateErr == nil {
			a.serverState = serverState
			a.serverStateKnown = true
		} else if a.lastError == "" {
			a.lastError = serverStateErr.Error()
		}
		a.refreshVisibleTorrents()
		a.updateTray()
	})
}

func (a *application) runBulkAction(label string, fn func(context.Context, []string) error) {
	hashes := a.selectedHashes()
	if len(hashes) == 0 {
		dialog.ShowInformation("No selection", "Select at least one torrent first.", a.window)
		return
	}

	a.runBulkActionForHashes(label, hashes, fn)
}

func (a *application) runBulkActionForHashes(label string, hashes []string, fn func(context.Context, []string) error) {
	go func() {
		err := fn(context.Background(), hashes)
		fyne.Do(func() {
			if err != nil {
				dialog.ShowError(fmt.Errorf("%s: %w", label, err), a.window)
				return
			}
			a.refreshNow()
		})
	}()
}

func (a *application) startSelectedTorrents() {
	a.runBulkAction("start torrents", func(ctx context.Context, hashes []string) error {
		return a.controller.StartTorrents(ctx, hashes)
	})
}

func (a *application) stopSelectedTorrents() {
	a.runBulkAction("stop torrents", func(ctx context.Context, hashes []string) error {
		return a.controller.StopTorrents(ctx, hashes)
	})
}

func (a *application) forceRecheckSelectedTorrents() {
	a.runBulkAction("force recheck torrents", func(ctx context.Context, hashes []string) error {
		return a.controller.ForceRecheckTorrents(ctx, hashes)
	})
}

func (a *application) forceReannounceSelectedTorrents() {
	a.runBulkAction("force reannounce torrents", func(ctx context.Context, hashes []string) error {
		return a.controller.ForceReannounceTorrents(ctx, hashes)
	})
}

func (a *application) openSetLocationDialog() {
	hashes := a.selectedHashes()
	if len(hashes) == 0 {
		dialog.ShowInformation("No selection", "Select at least one torrent first.", a.window)
		return
	}

	cfg := a.controller.Config()
	status := widget.NewLabel("")
	status.Hide()
	setStatus := func(message string) {
		status.SetText(message)
		if strings.TrimSpace(message) == "" {
			status.Hide()
			return
		}
		status.Show()
	}

	locationEntry := newPathAutocompleteEntry(
		cfg.UI.RecentSavePaths,
		a.controller.SuggestDirectories,
		setStatus,
	)
	locationEntry.SetText(a.commonSelectedSavePath(hashes))

	content := container.NewVBox(
		widget.NewForm(widget.NewFormItem("Save location", locationEntry)),
		status,
	)
	setLocationDialog := dialog.NewCustomConfirm("Set location", "Apply", "Cancel", content, func(ok bool) {
		if !ok {
			locationEntry.Close()
			return
		}
		location := locationEntry.Text
		locationEntry.Close()
		a.runBulkActionForHashes("set torrent location", hashes, func(ctx context.Context, hashes []string) error {
			return a.controller.SetTorrentLocation(ctx, hashes, location)
		})
	}, a.window)
	setLocationDialog.SetOnClosed(func() {
		locationEntry.Close()
	})
	setLocationDialog.Resize(relativeDialogSize(a.window.Canvas().Size(), setLocationDialog.MinSize(), 0.85))
	setLocationDialog.Show()
}

func (a *application) openRenameTorrentDialog() {
	torrent, ok := a.selectedRenameTarget()
	if !ok {
		dialog.ShowInformation("No torrent selected", "Select exactly one torrent first.", a.window)
		return
	}

	hash := torrent.Hash
	nameEntry := widget.NewEntry()
	nameEntry.SetText(torrent.Name)

	content := widget.NewForm(widget.NewFormItem("Name", nameEntry))
	renameDialog := dialog.NewCustomConfirm("Rename torrent", "Rename", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		name := nameEntry.Text
		go func() {
			err := a.controller.RenameTorrent(context.Background(), hash, name)
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(fmt.Errorf("rename torrent: %w", err), a.window)
					return
				}
				a.refreshNow()
			})
		}()
	}, a.window)
	renameDialog.Resize(relativeDialogSize(a.window.Canvas().Size(), renameDialog.MinSize(), 0.5))
	renameDialog.Show()
}

func relativeDialogSize(parent fyne.Size, min fyne.Size, widthRatio float32) fyne.Size {
	if parent.Width <= 0 || widthRatio <= 0 {
		return min
	}

	width := parent.Width * widthRatio
	if width < min.Width {
		width = min.Width
	}

	return fyne.NewSize(width, min.Height)
}

func (a *application) copySelectedTorrentNames() {
	content, ok := a.selectedTorrentNamesText()
	if !ok || a.fyApp == nil {
		return
	}
	a.fyApp.Clipboard().SetContent(content)
}

func (a *application) copySelectedTorrentMagnetLinks() {
	content, ok := a.selectedTorrentMagnetLinksText()
	if !ok || a.fyApp == nil {
		return
	}
	a.fyApp.Clipboard().SetContent(content)
}

func (a *application) selectedTorrentNamesText() (string, bool) {
	hashes := a.selectedHashes()
	if len(hashes) == 0 {
		return "", false
	}

	names := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		torrent, ok := a.findTorrentByHash(hash)
		if !ok {
			continue
		}
		names = append(names, torrent.Name)
	}
	if len(names) == 0 {
		return "", false
	}

	return strings.Join(names, "\n"), true
}

func (a *application) selectedTorrentMagnetLinksText() (string, bool) {
	hashes := a.selectedHashes()
	if len(hashes) == 0 {
		return "", false
	}

	links := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		torrent, ok := a.findTorrentByHash(hash)
		if !ok || strings.TrimSpace(torrent.MagnetURI) == "" {
			continue
		}
		links = append(links, torrent.MagnetURI)
	}
	if len(links) == 0 {
		return "", false
	}

	return strings.Join(links, "\n"), true
}

func (a *application) commonSelectedSavePath(hashes []string) string {
	var common string
	for _, hash := range hashes {
		torrent, ok := a.findTorrentByHash(hash)
		if !ok {
			return ""
		}
		savePath := strings.TrimSpace(torrent.SavePath)
		if savePath == "" {
			return ""
		}
		if common == "" {
			common = savePath
			continue
		}
		if savePath != common {
			return ""
		}
	}
	return common
}

func (a *application) selectedRenameTarget() (qbt.Torrent, bool) {
	if len(a.selection) != 1 {
		return qbt.Torrent{}, false
	}

	for hash := range a.selection {
		return a.findTorrentByHash(hash)
	}

	return qbt.Torrent{}, false
}

func (a *application) confirmDelete() {
	hashes := a.selectedHashes()
	if len(hashes) == 0 {
		dialog.ShowInformation("No selection", "Select at least one torrent first.", a.window)
		return
	}

	deleteFiles := widget.NewCheck("Also remove content files", nil)
	message := fmt.Sprintf("Remove %d selected torrents?", len(hashes))
	if len(hashes) == 1 {
		if torrent, ok := a.findTorrentByHash(hashes[0]); ok {
			message = fmt.Sprintf("Remove torrent %q?", torrent.Name)
		}
	}

	content := container.NewVBox(
		widget.NewLabel(message),
		deleteFiles,
	)

	confirm := dialog.NewCustomConfirm("Remove torrents", "Remove", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		go func() {
			err := a.controller.DeleteTorrents(context.Background(), hashes, deleteFiles.Checked)
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(err, a.window)
					return
				}
				for _, hash := range hashes {
					delete(a.selection, hash)
				}
				if len(a.selection) == 0 {
					a.selectionAnchor = ""
				}
				a.refreshNow()
			})
		}()
	}, a.window)
	confirm.Show()
}

func (a *application) applyTorrentSelection(hash string, modifier fyne.KeyModifier) {
	if strings.TrimSpace(hash) == "" {
		return
	}

	switch {
	case modifier&fyne.KeyModifierShift != 0:
		a.selectTorrentRange(hash)
	case modifier&(fyne.KeyModifierControl|fyne.KeyModifierSuper) != 0:
		a.toggleTorrentSelection(hash)
	default:
		a.selectOnlyTorrent(hash)
	}
	a.refreshTorrentSelection()
}

func (a *application) prepareTorrentContextSelection(hash string) {
	if strings.TrimSpace(hash) == "" {
		return
	}
	if a.selection[hash] {
		return
	}
	a.selectOnlyTorrent(hash)
	a.refreshTorrentSelection()
}

func (a *application) selectOnlyTorrent(hash string) {
	a.selection = map[string]bool{hash: true}
	a.selectionAnchor = hash
}

func (a *application) toggleTorrentSelection(hash string) {
	if a.selection == nil {
		a.selection = map[string]bool{}
	}
	if a.selection[hash] {
		delete(a.selection, hash)
		if len(a.selection) == 0 {
			a.selectionAnchor = ""
			return
		}
		if a.selectionAnchor == hash {
			a.selectionAnchor = ""
			for _, torrent := range a.visibleTorrents {
				if a.selection[torrent.Hash] {
					a.selectionAnchor = torrent.Hash

					break
				}
			}
		}
		return
	}
	a.selection[hash] = true
	a.selectionAnchor = hash
}

func (a *application) selectTorrentRange(hash string) {
	anchorIndex := a.visibleTorrentIndex(a.selectionAnchor)
	targetIndex := a.visibleTorrentIndex(hash)
	if anchorIndex < 0 || targetIndex < 0 {
		a.selectOnlyTorrent(hash)
		return
	}
	if a.selection == nil {
		a.selection = map[string]bool{}
	}
	clear(a.selection)
	if anchorIndex > targetIndex {
		anchorIndex, targetIndex = targetIndex, anchorIndex
	}
	for index := anchorIndex; index <= targetIndex; index++ {
		a.selection[a.visibleTorrents[index].Hash] = true
	}
}

func (a *application) visibleTorrentIndex(hash string) int {
	if strings.TrimSpace(hash) == "" {
		return -1
	}
	for index, torrent := range a.visibleTorrents {
		if torrent.Hash == hash {
			return index
		}
	}
	return -1
}

func (a *application) pruneSelectionToVisible() {
	if len(a.selection) == 0 {
		a.selectionAnchor = ""
		return
	}
	visible := make(map[string]struct{}, len(a.visibleTorrents))
	for _, torrent := range a.visibleTorrents {
		visible[torrent.Hash] = struct{}{}
	}
	for hash := range a.selection {
		if _, ok := visible[hash]; ok {
			continue
		}
		delete(a.selection, hash)
	}
	if len(a.selection) == 0 {
		a.selectionAnchor = ""
		return
	}
	if _, ok := visible[a.selectionAnchor]; !ok {
		a.selectionAnchor = ""
		for _, torrent := range a.visibleTorrents {
			if a.selection[torrent.Hash] {
				a.selectionAnchor = torrent.Hash

				break
			}
		}
	}
}

func (a *application) refreshTorrentSelection() {
	if a.list != nil {
		a.list.Refresh()
	}
}

func (a *application) selectedHashes() []string {
	if len(a.selection) == 0 {
		return nil
	}

	hashes := make([]string, 0, len(a.selection))
	seen := make(map[string]struct{}, len(a.selection))
	for _, torrent := range a.visibleTorrents {
		if !a.selection[torrent.Hash] {
			continue
		}
		hashes = append(hashes, torrent.Hash)
		seen[torrent.Hash] = struct{}{}
	}
	for _, torrent := range a.allTorrents {
		if !a.selection[torrent.Hash] {
			continue
		}
		if _, ok := seen[torrent.Hash]; ok {
			continue
		}
		hashes = append(hashes, torrent.Hash)
		seen[torrent.Hash] = struct{}{}
	}
	return hashes
}

func (a *application) findTorrentByHash(hash string) (qbt.Torrent, bool) {
	for _, torrent := range a.allTorrents {
		if torrent.Hash == hash {
			return torrent, true
		}
	}
	return qbt.Torrent{}, false
}

func (a *application) updateTray() {
	if !a.trayAvailable {
		return
	}
	label := fmt.Sprintf("Down %s | Up %s", appcore.HumanSpeed(a.transfer.DownloadSpeed), appcore.HumanSpeed(a.transfer.UploadSpeed))
	fyne.Do(func() {
		a.trayState.speedItem.Label = label
		a.trayState.desktopApp.SetSystemTrayMenu(fyne.NewMenu(appcore.Name, a.trayState.speedItem, a.trayState.showItem, a.trayState.quitItem))
		systray.SetTooltip(label)
	})
}

func statusColor(state string) color.Color {
	switch appcore.StatusLabel(state) {
	case "Downloading":
		return color.NRGBA{R: 0x0b, G: 0x84, B: 0xf3, A: 0xff}
	case "Seeding", "Completed":
		return color.NRGBA{R: 0x20, G: 0x96, B: 0x4c, A: 0xff}
	case "Paused":
		return color.NRGBA{R: 0x7c, G: 0x85, B: 0x90, A: 0xff}
	case "Checking", "Metadata", "Queued":
		return color.NRGBA{R: 0xe0, G: 0x9f, B: 0x1f, A: 0xff}
	case "Error", "Missing files":
		return color.NRGBA{R: 0xc9, G: 0x2a, B: 0x2a, A: 0xff}
	default:
		return color.NRGBA{R: 0x49, G: 0x5a, B: 0x6a, A: 0xff}
	}
}

func optionalNumberValidator(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return numberValidator(raw)
}

func numberValidator(raw string) error {
	_, err := parsePositiveInt(raw)
	return err
}

func parsePositiveInt(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("value is required")
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("must be a whole number")
	}
	if n <= 0 {
		return 0, fmt.Errorf("must be greater than zero")
	}
	return n, nil
}

func mergeUnique(primary []string, secondary []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(primary)+len(secondary))
	for _, item := range append(primary, secondary...) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func sortColumnLabels() []string {
	return []string{
		"Name",
		"Size",
		"Progress",
		"Status",
		"Download speed",
		"Upload speed",
		"ETA",
		"Added",
	}
}

func sortColumnLabel(key string) string {
	switch strings.TrimSpace(key) {
	case "name":
		return "Name"
	case "size":
		return "Size"
	case "progress":
		return "Progress"
	case "status":
		return "Status"
	case "down":
		return "Download speed"
	case "up":
		return "Upload speed"
	case "eta":
		return "ETA"
	case "added":
		return "Added"
	default:
		return "Added"
	}
}

func sortColumnKey(label string) string {
	switch strings.TrimSpace(label) {
	case "Name":
		return "name"
	case "Size":
		return "size"
	case "Progress":
		return "progress"
	case "Status":
		return "status"
	case "Download speed":
		return "down"
	case "Upload speed":
		return "up"
	case "ETA":
		return "eta"
	case "Added":
		return "added"
	default:
		return config.Default().UI.SortColumn
	}
}
