// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package tui

import (
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/flokiorg/twallet/load"
	page "github.com/flokiorg/twallet/pages"
	. "github.com/flokiorg/twallet/shared"
)

const (
	splashScreenDelay = time.Second * 2
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
	pages *tview.Pages
	load  *load.Load
}

func NewApp(appInfo *load.AppInfo, wallet Wallet) *App {
	app := &App{
		Application: tview.NewApplication(),
		pages:       tview.NewPages(),
	}

	app.load = load.NewLoad(appInfo, wallet, app.Application, app.pages)
	app.EnablePaste(true).EnableMouse(true)

	app.pages.AddPage("splashscreen", page.SplashScreen(), true, true).
		AddPage("reloading", page.ReloadingScreen(), true, false)

	app.SetRoot(app.pages, true).SetFocus(app.pages)

	time.AfterFunc(splashScreenDelay, func() {
		app.QueueUpdateDraw(func() {
			app.pages.AddAndSwitchToPage("main", page.NewEntrypoint(app.load), true)
		})
	})

	return app
}
