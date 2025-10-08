// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/flokiorg/flnd/flnwallet"
	"github.com/flokiorg/twallet/config"
	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/pages"
	"github.com/flokiorg/twallet/utils"
)

const (
	splashScreenDelay = time.Second * 1
)

func init() {
	// tview.Borders.HorizontalFocus = tview.BoxDrawingsHeavyHorizontal
	// tview.Borders.VerticalFocus = tview.BoxDrawingsHeavyVertical
	// tview.Borders.TopLeftFocus = tview.BoxDrawingsHeavyDownAndRight
	// tview.Borders.TopRightFocus = tview.BoxDrawingsHeavyDownAndLeft
	// tview.Borders.BottomLeftFocus = tview.BoxDrawingsHeavyUpAndRight
	// tview.Borders.BottomRightFocus = tview.BoxDrawingsHeavyUpAndLeft

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
		ContrastSecondaryTextColor:  tcell.ColorNavy,
	}
}

type App struct {
	*tview.Application
	pages  *tview.Pages
	cfg    *config.AppConfig
	flnsvc *flnwallet.Service
}

func NewApp(cfg *config.AppConfig) *App {
	app := &App{
		Application: tview.NewApplication(),
		pages:       tview.NewPages(),
		cfg:         cfg,
	}

	app.EnablePaste(true).EnableMouse(true)

	bootText, splashscreen := pages.SplashScreen(app.Application)
	app.pages.AddPage("splashscreen", splashscreen, true, true).
		AddPage("reloading", pages.ReloadingScreen(), true, false)

	app.SetRoot(app.pages, true).SetFocus(app.pages)

	go app.init(bootText)

	return app
}

func (app *App) init(bootText chan<- string) {

	defer close(bootText)
	app.flnsvc = flnwallet.New(context.Background(), &app.cfg.ServiceConfig)

	sub := app.flnsvc.Subscribe()
	defer app.flnsvc.Unsubscribe(sub)

loop:
	for u := range sub {
		switch u.State {
		case flnwallet.StatusNone, flnwallet.StatusInit:

		case flnwallet.StatusDown:
			select {
			case bootText <- fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", utils.FormatBootError(u.Err)):
			default:
			}
			app.flnsvc.Stop()
			return

		case flnwallet.StatusQuit:
			app.Stop()
			return

		default:
			break loop
		}
	}

	time.AfterFunc(splashScreenDelay, func() {
		app.QueueUpdateDraw(func() {
			loader := load.NewLoad(app.cfg, app.flnsvc, app.Application, app.pages)
			app.pages.AddAndSwitchToPage("main", pages.NewEntrypoint(loader), true)
		})
	})

}

func (app *App) Close() {
	if app.flnsvc != nil {
		app.flnsvc.Stop()
	}
}
