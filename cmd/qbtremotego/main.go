package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"

	appcore "github.com/skobkin/qbtremotego/internal/app"
	"github.com/skobkin/qbtremotego/internal/platform"
	"github.com/skobkin/qbtremotego/internal/ui"
)

const alreadyRunningMessage = "qBtRemoteGo is already running for this user.\nClose the existing instance before starting another copy."

func main() {
	if err := run(); err != nil {
		slog.Error("run app", "error", err)
		os.Exit(1)
	}
}

func run() error {
	initialInvocation := appcore.ParseInvocationArgs(os.Args[1:])

	slog.Info("acquiring single-instance lock", "app_id", appcore.ID)
	instanceLock, err := platform.AcquireInstanceLock(appcore.ID)
	if err != nil {
		if errors.Is(err, platform.ErrInstanceAlreadyRunning) {
			slog.Warn("single-instance lock contention: another app instance is already running", "app_id", appcore.ID)
			if !initialInvocation.Empty() {
				if err := platform.ForwardActivation(appcore.ID, os.Args[1:]); err != nil {
					showActivationFailureDialog(err)

					return fmt.Errorf("forward activation: %w", err)
				}

				return nil
			}
			showAlreadyRunningDialog(alreadyRunningMessage)

			return fmt.Errorf("acquire instance lock: %w", err)
		}
		if errors.Is(err, platform.ErrInstanceLockUnsupported) {
			slog.Warn("single-instance lock is not supported on this platform; continuing without lock", "error", err)
		} else {
			return fmt.Errorf("acquire instance lock: %w", err)
		}
	} else {
		slog.Info("single-instance lock acquired", "app_id", appcore.ID)
		defer func() {
			if err := instanceLock.Release(); err != nil {
				slog.Warn("release instance lock", "error", err)
			}
		}()
	}

	activations := make(chan appcore.InvocationBatch, 8)
	activationServer, err := platform.StartActivationServer(appcore.ID, slog.Default(), func(args []string) {
		batch := appcore.ParseInvocationArgs(args)
		if batch.Empty() {
			return
		}
		activations <- batch
	})
	if err != nil {
		return fmt.Errorf("start activation server: %w", err)
	}
	defer func() {
		if err := activationServer.Close(); err != nil {
			slog.Warn("close activation server", "error", err)
		}
	}()

	if err := ui.Run(initialInvocation, activations); err != nil {
		return fmt.Errorf("run ui: %w", err)
	}

	return nil
}

func showActivationFailureDialog(err error) {
	message := fmt.Sprintf("qBtRemoteGo is already running, but the handler request could not be delivered.\n\n%v", err)
	_, _ = fmt.Fprintln(os.Stderr, message)

	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Warn("show activation failure dialog", "error", recovered)
		}
	}()

	fyApp := fyneapp.New()
	window := fyApp.NewWindow(appcore.Name)
	window.Resize(fyne.NewSize(520, 200))

	var quitOnce sync.Once
	quit := func() {
		quitOnce.Do(func() {
			fyApp.Quit()
		})
	}

	info := dialog.NewError(err, window)
	info.SetOnClosed(quit)
	window.SetCloseIntercept(quit)
	window.Show()
	info.Show()
	fyApp.Run()
}

func showAlreadyRunningDialog(message string) {
	_, _ = fmt.Fprintln(os.Stderr, message)

	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Warn("show already-running dialog", "error", recovered)
		}
	}()

	fyApp := fyneapp.New()
	window := fyApp.NewWindow(appcore.Name)
	window.Resize(fyne.NewSize(500, 160))

	var quitOnce sync.Once
	quit := func() {
		quitOnce.Do(func() {
			fyApp.Quit()
		})
	}

	info := dialog.NewInformation(appcore.Name+" is already running", message, window)
	info.SetOnClosed(quit)
	window.SetCloseIntercept(quit)
	window.Show()
	info.Show()
	fyApp.Run()
}
