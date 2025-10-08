// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package pages

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"

	. "github.com/flokiorg/twallet/shared"
)

func logoView() tview.Primitive {

	splashLogo := tview.NewTextView().
		SetText(strings.ReplaceAll(SPLASH_LOGO_TEXT, "X", "[orange:-:-]|[-:-:-]")).
		SetDynamicColors(true)

	logoRow := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(splashLogo, 7, 1, false).
		AddItem(nil, 0, 1, false)

	view := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(nil, 0, 1, false).
		AddItem(logoRow, 24, 1, false).
		AddItem(nil, 0, 1, false)

	return view
}

func SplashScreen(app *tview.Application) (chan<- string, tview.Primitive) {

	welcomeText := tview.NewTextView().
		SetText(WELCOME_MESSAGE).
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	welcomRow := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(welcomeText, 1, 1, false).
		AddItem(nil, 0, 1, false)

	bootTextField := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	bootTextCentered := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(nil, 0, 1, false).
		AddItem(bootTextField, 80, 0, false).
		AddItem(nil, 0, 1, false)

	bootTextRow := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(bootTextCentered, 5, 1, false).
		AddItem(nil, 0, 1, false)

	view := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(logoView(), 9, 1, false).
		AddItem(welcomRow, 1, 1, false).
		AddItem(nil, 0, 1, false).
		AddItem(bootTextRow, 1, 1, false).
		AddItem(nil, 0, 1, false)

	bootText := make(chan string)
	go func() {
		for t := range bootText {
			app.QueueUpdateDraw(func() {
				bootTextField.SetText(t)
			})
		}
	}()

	return bootText, view
}

func ReloadingScreen() *tview.Flex {

	text := tview.NewTextView().
		SetText(fmt.Sprintf("[-:-:-] %s", "loading...")).
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)

	rootFlex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(logoView(), 9, 1, false).
		AddItem(text, 1, 1, false).
		AddItem(nil, 0, 1, false)

	return rootFlex

}
