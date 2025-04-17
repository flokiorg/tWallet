// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package root

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"

	"github.com/flokiorg/go-flokicoin/wire"
	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/shared"
	. "github.com/flokiorg/twallet/shared"
	"github.com/flokiorg/twallet/utils"
	"github.com/gdamore/tcell/v2"
)

type Header struct {
	*tview.Flex
	logo    *tview.TextView
	balance *tview.TextView
	load    *load.Load
	destroy chan struct{}
}

func NewHeader(l *load.Load) *Header {
	h := &Header{
		Flex:    tview.NewFlex(),
		load:    l,
		destroy: make(chan struct{}),
	}

	h.logo = h.buildLogo()
	h.AddItem(h.logo, 0, 1, false)

	if l.Wallet.IsOpened() {

		h.balance = tview.NewTextView().
			SetText(fmt.Sprintf("Balance: [%s:-:b]%s", tcell.ColorGreen, shared.FormatAmountView(h.load.Wallet.Balance(), 6))).
			SetDynamicColors(true).
			SetTextColor(tcell.ColorOrange).
			SetTextAlign(tview.AlignLeft)

		hotkeys := tview.NewTextView().
			SetDynamicColors(true).
			SetTextAlign(tview.AlignLeft)

		fmt.Fprintf(hotkeys, "[%s:-:b]<s> [white:-:-]Send\n", tcell.ColorLightSkyBlue)
		fmt.Fprintf(hotkeys, "[%s:-:b]<r> [white:-:-]Receive", tcell.ColorLightSkyBlue)

		walletInfo := tview.NewGrid().
			SetRows(1, 1, 1, 2).SetColumns(0)

		walletInfo.AddItem(h.balance, 1, 0, 1, 1, 0, 0, false).
			AddItem(hotkeys, 3, 0, 1, 1, 0, 0, false)

		h.AddItem(walletInfo, 30, 1, false)

		go h.updates()
	}

	return h
}

func (h *Header) updates() {

	notifSubscription := h.load.Notif.Subscribe()

	for {

		select {
		case <-notifSubscription:
			balance := h.load.Wallet.Balance()
			h.updateBalance(balance)
			h.load.Logger.Trace().
				Str("event", "header_notification").
				Float64("new_balance", balance).
				Msg("Received header notification")
		case <-h.destroy:
			return
		}
	}
}

func (h *Header) Destroy() {
	close(h.destroy)
}

func (h *Header) updateBalance(balance float64) {
	h.load.Application.QueueUpdateDraw(func() {
		h.balance.SetText(fmt.Sprintf("Balance: [%s:-:b]%s", tcell.ColorGreen, shared.FormatAmountView(balance, 6)))
	})
}

func (h *Header) buildLogo() *tview.TextView {

	logo := tview.NewTextView().SetDynamicColors(true)
	logo.SetBorder(false)

	var logoColor tcell.Color

	switch h.load.Params.Network.Net {
	case wire.MainNet:
		logoColor = tcell.ColorOrange
	case wire.TestNet3:
		logoColor = tcell.ColorRed
	default:
		logoColor = tcell.ColorYellowGreen
	}

	lines := strings.Split(LOGO_TEXT, "\n")
	fmt.Fprintf(logo, "[%s:-:-]", logoColor)
	for i := 1; i < len(lines); i++ {
		fmt.Fprintf(logo, "   [%s::b]%s", "", lines[i])
		fmt.Fprintf(logo, "\n")
	}

	version := fmt.Sprintf("\t v%s", utils.Version)
	fmt.Fprint(logo, version)
	return logo
}
