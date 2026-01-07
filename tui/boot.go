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

	"github.com/gdamore/tcell/v2"

	"github.com/flokiorg/twallet/flnd"
	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/pages"
	"github.com/flokiorg/twallet/utils"
)

const (
	startupHealthTimeout = 20 * time.Second
	purgeRetryAttempts   = 3
	purgeRetryDelay      = 5 * time.Second
)

func (app *App) startBoot() {
	bootText, splashscreen := pages.SplashScreen(app.Application)
	app.bootLog = bootText
	app.pages.AddPage("splashscreen", splashscreen, true, true).
		AddPage("reloading", pages.ReloadingScreen(), true, false)

	app.SetRoot(app.pages, true).SetFocus(app.pages)

	go app.bootLoop()

	if app.autoRecover {
		go app.requestRecovery()
	}
}

func (app *App) bootLoop() {
	defer func() {
		if r := recover(); r != nil {
			msg := "startup panic"
			switch v := r.(type) {
			case error:
				msg = v.Error()
			case string:
				msg = v
			default:
				msg = fmt.Sprint(v)
			}
			app.log(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", msg))
		}
	}()

	time.Sleep(time.Second * 1)
bootLoop:
	for {
		if app.autoRecover || app.consumeRecoveryRequest() {
			app.autoRecover = false
			if err := app.recoverWallet("Recovery requested"); err != nil {
				app.stopService()
				return
			}
			continue
		}

		if app.flnsvc == nil {
			app.flnsvc = flnd.New(context.Background(), &app.cfg.ServiceConfig)
		}

		sub := app.flnsvc.Subscribe()

		for {
			select {
			case <-app.recoveryRequests:
				app.flnsvc.Unsubscribe(sub)
				if err := app.recoverWallet("Recovery requested"); err != nil {
					app.stopService()
					return
				}
				continue bootLoop

			case update, ok := <-sub:
				if !ok || update == nil {
					app.log("[red:-:-]Error:[-:-:-] wallet service closed unexpectedly during startup")
					app.stopService()
					return
				}

				switch update.State {
				case flnd.StatusNone, flnd.StatusInit:
					continue
				case flnd.StatusDown:
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
						app.log(combinedMsg)
						app.stopService()
						if app.waitForRecoveryConfirmation() {
							if err := app.recoverWallet("Neutrino headers failed to load during startup"); err != nil {
								app.stopService()
								return
							}
							continue bootLoop
						}
						app.log("[red]Recovery cancelled. Exiting startup.")
						app.stopService()
						return
					}
					app.flnsvc.Unsubscribe(sub)
					app.log(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s. Press Ctrl+C to quit. If it keeps happening, reach out to the Lokichain community.", msg))
					app.stopService()
					return
				case flnd.StatusQuit:
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
		app.scheduleRecoveryRestart()
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

func (app *App) waitForRecoveryConfirmation() bool {
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

func (app *App) log(msg string) {
	if strings.TrimSpace(msg) == "" {
		return
	}
	defer func() {
		if recover() != nil {
			// Channel closed; drop the log to avoid crashing during shutdown.
		}
	}()
	if app.bootLog != nil {
		app.bootLog <- msg
	}
}

func (app *App) scheduleRecoveryRestart() {
	app.restartRecovery = true
	app.log("[orange]Restarting in recovery mode…")
	app.Stop()
}

func (app *App) recoverWallet(reason string) error {
	app.clearRecoveryRequests()

	if reason != "" {
		app.log(fmt.Sprintf("[orange]Entering recovery mode: %s", reason))
	} else {
		app.log("[orange]Entering recovery mode…")
	}

	app.log("[gray]Stopping wallet service…")
	app.stopService()

	app.log("[gray]Clearing cached chain data…")
	var purgeErr error
	for attempt := 1; attempt <= purgeRetryAttempts; attempt++ {
		purgeErr = load.PurgeNeutrinoCache(app.cfg, func(msg string) {
			app.log(fmt.Sprintf("[gray]%s", msg))
		})
		if purgeErr == nil {
			break
		}

		if attempt < purgeRetryAttempts {
			app.log(fmt.Sprintf("[orange]Cache cleanup failed (attempt %d/%d): %s. Retrying in 5s…", attempt, purgeRetryAttempts, utils.FormatBootError(purgeErr)))
			time.Sleep(purgeRetryDelay)
		}
	}
	if purgeErr != nil {
		app.log(fmt.Sprintf("[red]Recovery failed: %s. Press Ctrl+C to quit. If it keeps happening, reach out to the Lokichain community.", utils.FormatBootError(purgeErr)))
		return purgeErr
	}

	app.log("[gray]Restarting wallet service…")
	app.flnsvc = flnd.New(context.Background(), &app.cfg.ServiceConfig)

	health, err := load.CheckWalletHealth(context.Background(), app.flnsvc, startupHealthTimeout)
	if err != nil {
		app.log(fmt.Sprintf("[red]Recovery failed during health check: %s", utils.FormatBootError(err)))
		return err
	}

	if !health.Healthy {
		reason := health.Reason
		if reason == "" {
			reason = "wallet still unavailable"
		}
		app.log(fmt.Sprintf("[red]Wallet still unhealthy after recovery: %s", reason))
		app.log("[red]Please restore from your seed/mnemonic and restart twallet. Press Ctrl+C to quit.")
		return errors.New("wallet remains unhealthy after recovery")
	}

	app.log("[green]Wallet recovered. Continuing startup…")
	return nil
}

func (app *App) launchMain() {
	app.QueueUpdateDraw(func() {
		loader := load.NewLoad(app.cfg, app.flnsvc, app.Application, app.pages)
		app.pages.AddAndSwitchToPage("main", pages.NewEntrypoint(loader), true)
	})
}
