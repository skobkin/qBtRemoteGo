package ui

import (
	"context"
	"fmt"
	"image/color"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"fyne.io/systray"

	appcore "github.com/skobkin/qbtremotego/internal/app"
	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/logging"
	"github.com/skobkin/qbtremotego/internal/qbt"
	"github.com/skobkin/qbtremotego/internal/resources"
)

type application struct {
	fyApp  fyne.App
	window fyne.Window
	logger *slog.Logger

	controller *appcore.Controller

	mu              sync.Mutex
	allTorrents     []qbt.Torrent
	visibleTorrents []qbt.Torrent
	selection       map[string]bool
	filterQuery     string
	transfer        qbt.TransferInfo
	lastError       string
	windowVisible   bool
	trayAvailable   bool

	list         *widget.List
	tableHeader  *torrentHeaderRow
	tablePreview *canvas.Rectangle
	tableScroll  *container.Scroll
	statusLabel  *widget.Label
	filterEntry  *widget.Entry
	filterBy     *widget.Select
	columnWidths map[string]float32
	previewX     float32

	trayState trayState

	filterTimer *time.Timer
}

type trayState struct {
	desktopApp desktop.App
	window     fyne.Window
	showItem   *fyne.MenuItem
	speedItem  *fyne.MenuItem
	quitItem   *fyne.MenuItem
}

func Run() error {
	configPath, err := config.DefaultConfigPath()
	if err != nil {
		return err
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	logManager, err := logging.New(cfg.Logging)
	if err != nil {
		return err
	}

	controller, err := appcore.NewController(configPath, logManager.Logger("controller"))
	if err != nil {
		return err
	}

	fyApp := app.NewWithID("github.com.skobkin.qbtremotego")
	fyApp.SetIcon(resources.AppIcon())

	window := fyApp.NewWindow("qBtRemoteGo")
	window.Resize(fyne.NewSize(1120, 680))
	window.SetIcon(resources.AppIcon())

	ui := &application{
		fyApp:         fyApp,
		window:        window,
		logger:        logManager.Logger("ui"),
		controller:    controller,
		selection:     map[string]bool{},
		windowVisible: true,
		statusLabel:   widget.NewLabel(""),
	}

	ui.buildMainWindow()
	ui.configureTray()
	ui.bindCloseBehavior()

	if prefill := controller.ParseInvocationArgs(os.Args[1:]); prefill != nil {
		fyne.Do(func() {
			ui.openAddWindow(prefill)
		})
	}

	startHidden := controller.Config().UI.StartMinimizedToTray && ui.trayAvailable
	if startHidden {
		ui.windowVisible = false
	} else {
		window.Show()
	}

	go ui.pollLoop()
	fyApp.Run()

	return nil
}

func (a *application) buildMainWindow() {
	addButton := widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() {
		a.openAddWindow(nil)
	})
	removeButton := widget.NewButtonWithIcon("Remove", theme.DeleteIcon(), func() {
		a.confirmDelete()
	})
	startButton := widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), func() {
		a.runBulkAction("start torrents", func(ctx context.Context, hashes []string) error {
			return a.controller.StartTorrents(ctx, hashes)
		})
	})
	stopButton := widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), func() {
		a.runBulkAction("stop torrents", func(ctx context.Context, hashes []string) error {
			return a.controller.StopTorrents(ctx, hashes)
		})
	})
	settingsButton := widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), func() {
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

	toolbar := container.NewBorder(nil, nil, leftTools, rightTools)

	center := a.buildTorrentTable()
	bottom := container.NewBorder(widget.NewSeparator(), nil, nil, nil, a.statusLabel)
	a.window.SetContent(container.NewBorder(toolbar, bottom, nil, nil, center))

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
		window:     a.window,
		speedItem:  fyne.NewMenuItem("Down 0 B/s | Up 0 B/s", nil),
		showItem: fyne.NewMenuItem("Open main window", func() {
			a.windowVisible = true
			a.window.Show()
			a.window.RequestFocus()
		}),
		quitItem: fyne.NewMenuItem("Quit application", func() {
			a.fyApp.Quit()
		}),
	}
	a.trayState.speedItem.Disabled = true
	desk.SetSystemTrayIcon(resources.TrayIcon())
	desk.SetSystemTrayWindow(a.window)
	desk.SetSystemTrayMenu(fyne.NewMenu("qBtRemoteGo", a.trayState.speedItem, a.trayState.showItem, a.trayState.quitItem))
	systray.SetTooltip("Down 0 B/s | Up 0 B/s")
}

func (a *application) openSettingsWindow() {
	cfg := a.controller.Config()

	win := a.fyApp.NewWindow("Settings")
	win.Resize(fyne.NewSize(620, 560))

	urlEntry := widget.NewEntry()
	urlEntry.SetText(cfg.Connection.URL)
	usernameEntry := widget.NewEntry()
	usernameEntry.SetText(cfg.Connection.Username)
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetText(cfg.Connection.Password)
	skipTLS := widget.NewCheck("", nil)
	skipTLS.SetChecked(cfg.Connection.SkipCertificateCheck)
	testStatus := widget.NewLabel("")

	rememberEntry := widget.NewEntry()
	rememberEntry.SetText(fmt.Sprintf("%d", cfg.UI.RememberPathCount))
	rememberEntry.Validator = numberValidator
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
	)

	uiForm := widget.NewForm(
		widget.NewFormItem("Number of paths to remember", rememberEntry),
		widget.NewFormItem("Path autocompletion", autocomplete),
		widget.NewFormItem("Torrent list update time (seconds)", activePoll),
		widget.NewFormItem("Background update time (seconds)", backgroundPoll),
		widget.NewFormItem("Start minimized to tray", startMinimized),
		widget.NewFormItem("Sort by", sortBy),
		widget.NewFormItem("Descending order", sortDescending),
	)

	integrationForm := widget.NewForm(
		widget.NewFormItem("Register magnet handler", registerMagnet),
		widget.NewFormItem("Register .torrent handler", registerTorrent),
		widget.NewFormItem("Start with the system", startWithSystem),
	)

	testButton := widget.NewButton("Test connection", func() {
		testStatus.SetText("Testing connection...")
		go func() {
			err := a.controller.TestConnection(context.Background(), config.ConnectionConfig{
				URL:                  urlEntry.Text,
				Username:             usernameEntry.Text,
				Password:             passwordEntry.Text,
				SkipCertificateCheck: skipTLS.Checked,
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
		updated.Connection.Username = usernameEntry.Text
		updated.Connection.Password = passwordEntry.Text
		updated.Connection.SkipCertificateCheck = skipTLS.Checked
		updated.UI.RememberPathCount = rememberCount
		updated.UI.PathAutocomplete = autocomplete.Checked
		updated.UI.ActivePollSeconds = activeSeconds
		updated.UI.BackgroundPollSeconds = backgroundSeconds
		updated.UI.StartMinimizedToTray = startMinimized.Checked
		updated.UI.SortColumn = sortColumnKey(sortBy.Selected)
		updated.UI.SortDescending = sortDescending.Checked
		updated.Integration.RegisterMagnetHandler = registerMagnet.Checked
		updated.Integration.RegisterTorrentHandler = registerTorrent.Checked
		updated.Integration.StartWithSystem = startWithSystem.Checked

		if err := a.controller.SaveConfig(updated); err != nil {
			dialog.ShowError(fmt.Errorf("settings saved with integration warnings:\n%w", err), win)
		}
		a.refreshVisibleTorrents()
		win.Close()
	})

	tabs := container.NewAppTabs(
		container.NewTabItem("Connection", container.NewPadded(container.NewVBox(
			connectionForm,
			testButton,
			testStatus,
		))),
		container.NewTabItem("UI", container.NewPadded(uiForm)),
		container.NewTabItem("Integration", container.NewPadded(integrationForm)),
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

func (a *application) openAddWindow(prefill *appcore.AddDialogPrefill) {
	win := a.fyApp.NewWindow("Add Torrent")
	win.Resize(fyne.NewSize(720, 720))

	cfg := a.controller.Config()
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
	} else if len(cfg.UI.RecentSavePaths) > 0 {
		data.SavePath = cfg.UI.RecentSavePaths[0]
	}

	categories, tags, preloadErr := a.controller.FetchCategoriesAndTags(context.Background())
	if preloadErr != nil {
		a.logger.Info("preload categories/tags", "error", preloadErr)
	}

	status := widget.NewLabel("")

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

	savePathEntry := widget.NewSelectEntry(cfg.UI.RecentSavePaths)
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
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				status.SetText(err.Error())
				return
			}
			if reader == nil {
				return
			}
			fileEntry.SetText(reader.URI().Path())
			_ = reader.Close()
		}, win)
	})

	suggestButton := widget.NewButton("Suggest", func() {
		status.SetText("Loading save path suggestions...")
		go func() {
			suggestions, err := a.controller.SuggestDirectories(context.Background(), savePathEntry.Text)
			fyne.Do(func() {
				if err != nil {
					status.SetText("Path suggestions unavailable: " + err.Error())
					return
				}
				if len(suggestions) == 0 {
					status.SetText("No matching save paths.")
					return
				}
				savePathEntry.SetOptions(mergeUnique(suggestions, cfg.UI.RecentSavePaths))
				status.SetText("Loaded path suggestions.")
			})
		}()
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

	form := widget.NewForm(
		widget.NewFormItem("Source type", sourceSelect),
		widget.NewFormItem("Source", sourceContainer),
		widget.NewFormItem("Torrent management mode", managementSelect),
		widget.NewFormItem("Save location", container.NewBorder(nil, nil, nil, suggestButton, savePathEntry)),
		widget.NewFormItem("Name override", renameEntry),
		widget.NewFormItem("Category", categoryEntry),
		widget.NewFormItem("Tags", tagsEntry),
		widget.NewFormItem("Start torrent", startCheck),
		widget.NewFormItem("Top of queue", topOfQueue),
		widget.NewFormItem("Stop condition", stopSelect),
		widget.NewFormItem("Skip hash check", skipHashCheck),
		widget.NewFormItem("Content layout", contentLayoutSelect),
		widget.NewFormItem("Download sequentially", sequential),
		widget.NewFormItem("Download first and last pieces first", firstLastPieces),
		widget.NewFormItem("Limit download rate (KiB/s)", downloadLimitEntry),
		widget.NewFormItem("Limit upload rate (KiB/s)", uploadLimitEntry),
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

		status.SetText("Submitting torrent...")
		submit.Disable()
		go func() {
			err := a.controller.AddTorrent(context.Background(), data)
			fyne.Do(func() {
				submit.Enable()
				if err != nil {
					status.SetText("Add torrent failed: " + err.Error())
					return
				}
				status.SetText("Torrent submitted.")
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
		container.NewVBox(status, container.NewHBox(layout.NewSpacer(), cancel, submit)),
		nil,
		nil,
		container.NewVScroll(form),
	)

	win.SetContent(content)
	win.Show()
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
	if a.list != nil {
		a.list.Refresh()
	}
	a.statusLabel.SetText(a.statusText())
}

func (a *application) statusText() string {
	parts := []string{
		fmt.Sprintf("Torrents: %d", len(a.allTorrents)),
		fmt.Sprintf("Visible: %d", len(a.visibleTorrents)),
		fmt.Sprintf("Down %s", appcore.HumanSpeed(a.transfer.DownloadSpeed)),
		fmt.Sprintf("Up %s", appcore.HumanSpeed(a.transfer.UploadSpeed)),
	}
	if strings.TrimSpace(a.lastError) != "" {
		parts = append(parts, "Last error: "+a.lastError)
	}
	return strings.Join(parts, " | ")
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
				a.refreshNow()
			})
		}()
	}, a.window)
	confirm.Show()
}

func (a *application) selectedHashes() []string {
	hashes := make([]string, 0, len(a.selection))
	for hash, selected := range a.selection {
		if selected {
			hashes = append(hashes, hash)
		}
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
	a.trayState.speedItem.Label = label
	a.trayState.desktopApp.SetSystemTrayMenu(fyne.NewMenu("qBtRemoteGo", a.trayState.speedItem, a.trayState.showItem, a.trayState.quitItem))
	systray.SetTooltip(label)
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
