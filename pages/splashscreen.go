// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package pages

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
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

func SplashScreen() tview.Primitive {

	welcomeText := tview.NewTextView().
		SetText(WELCOME_MESSAGE).
		SetDynamicColors(true).SetTextAlign(tview.AlignCenter)

	textRow := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(welcomeText, 1, 1, false).
		AddItem(nil, 0, 1, false)

	view := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(logoView(), 9, 1, false).
		AddItem(textRow, 1, 1, false).
		AddItem(nil, 0, 1, false)

	return view
}

func ReloadingScreen() *tview.Flex {

	logoBytes, _ := os.ReadFile(LOGO_TEXT)
	graphics, _ := png.Decode(bytes.NewReader(logoBytes))
	logo := tview.NewImage()
	logo.SetImage(graphics).SetColors(tview.TrueColor)

	text := tview.NewTextView().
		SetText(fmt.Sprintf("[-:-:-] %s", "loading...")).
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)

	centerRow := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(nil, 0, 1, false).
		AddItem(logo, 0, 1, false).
		AddItem(nil, 0, 1, false)

	rootFlex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(centerRow, 0, 1, false).
		AddItem(text, 1, 1, false).
		AddItem(nil, 0, 1, false)

	return rootFlex

}
