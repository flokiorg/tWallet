// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/flokiorg/flnd/flnwallet"
	"github.com/gdamore/tcell/v2"

	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/pages"
	"github.com/flokiorg/twallet/utils"
)

const (
	startupHealthTimeout = 20 * time.Second
)

func (app *App) startBoot() {
	bootText, splashscreen := pages.SplashScreen(app.Application)
	app.pages.AddPage("splashscreen", splashscreen, true, true).
		AddPage("reloading", pages.ReloadingScreen(), true, false)

	app.SetRoot(app.pages, true).SetFocus(app.pages)

	go app.bootLoop(bootText)
}

func (app *App) bootLoop(bootText chan<- string) {
	defer close(bootText)

	time.Sleep(time.Second * 1)
bootLoop:
	for {
		if app.flnsvc == nil {
			app.flnsvc = flnwallet.New(context.Background(), &app.cfg.ServiceConfig)
		}

		sub := app.flnsvc.Subscribe()

		for {
			select {
			case <-app.recoveryRequests:
				app.flnsvc.Unsubscribe(sub)
				if err := app.recoverWallet(bootText, "Recovery requested"); err != nil {
					app.stopService()
					return
				}
				continue bootLoop

			case update, ok := <-sub:
				if !ok || update == nil {
					app.sendBootNotification(bootText, "[red:-:-]Error:[-:-:-] wallet service closed unexpectedly during startup")
					app.stopService()
					return
				}

				switch update.State {
				case flnwallet.StatusNone, flnwallet.StatusInit:
					continue
				case flnwallet.StatusDown:
					msg := "wallet reported down during startup"
					if update.Err != nil {
						msg = utils.FormatBootError(update.Err)
					}
					if app.isEOFError(update.Err) {
						app.flnsvc.Unsubscribe(sub)
						combinedMsg := fmt.Sprintf(
							"[red:-:-]Error:[-:-:-] %s\n[orange]Neutrino headers look corrupted. Press 'r' to run recovery.\nPress Ctrl+C to quit.",
							msg,
						)
						app.sendBootNotification(bootText, combinedMsg)
						app.stopService()
						if app.waitForRecoveryConfirmation(bootText) {
							if err := app.recoverWallet(bootText, "Neutrino headers failed to load during startup"); err != nil {
								app.stopService()
								return
							}
							continue bootLoop
						}
						app.sendBootNotification(bootText, "[red]Recovery cancelled. Exiting startup.")
						app.stopService()
						return
					}
					app.flnsvc.Unsubscribe(sub)
					app.sendBootNotification(bootText, fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", msg))
					app.stopService()
					return
				case flnwallet.StatusQuit:
					app.stopService()
					return
				default:
					app.flnsvc.Unsubscribe(sub)
					app.launchMain()
					return
				}
			}
		}
	}
}

func (app *App) captureStartupKeys(event *tcell.EventKey) *tcell.EventKey {
	switch event.Rune() {
	case 'r', 'R':
		app.requestRecovery()
		return nil
	}
	return event
}

func (app *App) requestRecovery() {
	select {
	case app.recoveryRequests <- struct{}{}:
	default:
	}
}

func (app *App) consumeRecoveryRequest() bool {
	select {
	case <-app.recoveryRequests:
		return true
	default:
		return false
	}
}

func (app *App) clearRecoveryRequests() {
	for app.consumeRecoveryRequest() {
	}
}

func (app *App) stopService() {
	if app.flnsvc != nil {
		app.flnsvc.Stop()
	}
}

func (app *App) isEOFError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	if strings.Contains(err.Error(), "EOF") {
		return true
	}

	return false
}

func (app *App) waitForRecoveryConfirmation(bootText chan<- string) bool {
	app.clearRecoveryRequests()
	for {
		select {
		case <-app.recoveryRequests:
			return true
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (app *App) sendBootNotification(bootText chan<- string, msg string) {
	if strings.TrimSpace(msg) == "" {
		return
	}
	select {
	case bootText <- msg:
	default:
	}
}

func (app *App) recoverWallet(bootText chan<- string, reason string) error {
	app.clearRecoveryRequests()

	if reason != "" {
		app.sendBootNotification(bootText, fmt.Sprintf("[orange]Entering recovery mode: %s", reason))
	} else {
		app.sendBootNotification(bootText, "[orange]Entering recovery mode…")
	}

	app.sendBootNotification(bootText, "[gray]Stopping wallet service…")
	app.stopService()

	app.sendBootNotification(bootText, "[gray]Clearing cached chain data…")
	if err := load.PurgeNeutrinoCache(app.cfg, func(msg string) {
		app.sendBootNotification(bootText, fmt.Sprintf("[gray]%s", msg))
	}); err != nil {
		app.sendBootNotification(bootText, fmt.Sprintf("[red]Recovery failed: %s", utils.FormatBootError(err)))
		return err
	}

	app.sendBootNotification(bootText, "[gray]Restarting wallet service…")
	app.flnsvc = flnwallet.New(context.Background(), &app.cfg.ServiceConfig)

	health, err := load.CheckWalletHealth(context.Background(), app.flnsvc, startupHealthTimeout)
	if err != nil {
		app.sendBootNotification(bootText, fmt.Sprintf("[red]Recovery failed during health check: %s", utils.FormatBootError(err)))
		return err
	}

	if !health.Healthy {
		reason := health.Reason
		if reason == "" {
			reason = "wallet still unavailable"
		}
		app.sendBootNotification(bootText, fmt.Sprintf("[red]Wallet still unhealthy after recovery: %s", reason))
		app.sendBootNotification(bootText, "[red]Please restore from your seed/mnemonic and restart twallet. Press Ctrl+C to quit.")
		return errors.New("wallet remains unhealthy after recovery")
	}

	app.sendBootNotification(bootText, "[green]Wallet recovered. Continuing startup…")
	return nil
}

func (app *App) launchMain() {
	app.QueueUpdateDraw(func() {
		loader := load.NewLoad(app.cfg, app.flnsvc, app.Application, app.pages)
		app.pages.AddAndSwitchToPage("main", pages.NewEntrypoint(loader), true)
	})
}
