// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"os"

	"github.com/flokiorg/flnd/flnwallet"
	"github.com/flokiorg/twallet/config"
)

func init() {
	tview.Borders.HorizontalFocus = tview.BoxDrawingsLightHorizontal
	tview.Borders.VerticalFocus = tview.BoxDrawingsLightVertical
	tview.Borders.TopLeftFocus = tview.BoxDrawingsLightDownAndRight
	tview.Borders.TopRightFocus = tview.BoxDrawingsLightDownAndLeft
	tview.Borders.BottomLeftFocus = tview.BoxDrawingsLightUpAndRight
	tview.Borders.BottomRightFocus = tview.BoxDrawingsLightUpAndLeft

	tview.Styles = tview.Theme{
		PrimitiveBackgroundColor:    tcell.ColorBlack,
		ContrastBackgroundColor:     tcell.ColorGray,
		MoreContrastBackgroundColor: tcell.ColorOrange,
		BorderColor:                 tcell.ColorWhite,
		TitleColor:                  tcell.ColorWhite,
		GraphicsColor:               tcell.ColorWhite,
		PrimaryTextColor:            tcell.ColorWhite,
		SecondaryTextColor:          tcell.ColorWhite,
		TertiaryTextColor:           tcell.ColorGreen,
		InverseTextColor:            tcell.ColorBlue,
		ContrastSecondaryTextColor:  tcell.ColorDarkSlateGray,
	}
}

type App struct {
	*tview.Application
	pages            *tview.Pages
	cfg              *config.AppConfig
	flnsvc           *flnwallet.Service
	recoveryRequests chan struct{}
	bootLog          chan string
	autoRecover      bool
	restartRecovery  bool
}

func NewApp(cfg *config.AppConfig) *App {
	app := &App{
		Application:      tview.NewApplication(),
		pages:            tview.NewPages(),
		cfg:              cfg,
		recoveryRequests: make(chan struct{}, 1),
		autoRecover:      os.Getenv("TWALLET_AUTO_RECOVER") == "1",
	}

	app.EnablePaste(true).EnableMouse(true)
	app.installResizeGuard()
	app.SetInputCapture(app.captureStartupKeys)

	app.startBoot()

	return app
}

func (app *App) Close() {
	if app.flnsvc != nil {
		app.flnsvc.Stop()
	}
}

func (app *App) ShouldRestartForRecovery() bool {
	return app.restartRecovery
}

// installResizeGuard skips drawing while the terminal is reporting tiny/zero
// dimensions, which can happen mid-resize and has been known to trigger
// panics inside tcell's drawing routines.
func (app *App) installResizeGuard() {
	var skipped bool
	app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		w, h := screen.Size()
		if w < 1 || h < 1 {
			skipped = true
			return true
		}
		if skipped {
			screen.Clear()
			skipped = false
		}
		return false
	})
}
