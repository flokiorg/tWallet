// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package root

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"

	"github.com/flokiorg/flnd/flnwallet"
	"github.com/flokiorg/flnd/lnrpc"
	"github.com/flokiorg/go-flokicoin/chainutil"
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

	if ok, _ := l.Wallet.WalletExists(); ok {

		h.balance = tview.NewTextView().
			SetDynamicColors(true).
			SetTextColor(tcell.ColorOrange).
			SetTextAlign(tview.AlignLeft)

		hotkeys := tview.NewTextView().
			SetDynamicColors(true).
			SetTextAlign(tview.AlignLeft)

		if ev := h.load.Wallet.GetLastEvent(); ev.State != flnwallet.StatusLocked {
			balance, err := h.load.Wallet.Balance()
			if err != nil {
				l.Logger.Error().Err(err).Msg("unable to fetch balance")
			}
			h.balance.SetText(balanceView(balance))
			fmt.Fprintf(hotkeys, "\n[%s:-:b]<s> [white:-:-]Send  ", tcell.ColorLightSkyBlue)
			fmt.Fprintf(hotkeys, "[%s:-:b]<r> [white:-:-]Receive", tcell.ColorLightSkyBlue)
		}

		walletInfo := tview.NewGrid().
			SetRows(1, 1, 1, 2).SetColumns(0)

		walletInfo.AddItem(h.balance, 1, 0, 2, 1, 0, 0, false).
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
			balance, err := h.load.Wallet.Balance()
			if err == nil {
				h.updateBalance(balance)
			}

		case <-h.destroy:
			return
		}
	}
}

func (h *Header) Destroy() {
	close(h.destroy)
}

func (h *Header) updateBalance(resp *lnrpc.WalletBalanceResponse) {
	h.load.Logger.Debug().Msgf("new balance confirmed:%v unconfirmed:%v", resp.ConfirmedBalance, resp.UnconfirmedBalance)
	h.load.Cache.SetBalance(resp)
	h.load.Application.QueueUpdateDraw(func() {
		h.balance.SetText(balanceView(resp))
	})
}

func (h *Header) buildLogo() *tview.TextView {

	logo := tview.NewTextView().SetDynamicColors(true)
	logo.SetBorder(false)

	netColor := NetworkColor(*h.load.AppConfig.Network)

	lines := strings.Split(LOGO_TEXT, "\n")
	fmt.Fprintf(logo, "[%s:-:-]", netColor)
	for i := 1; i < len(lines); i++ {
		fmt.Fprintf(logo, "   [%s::b]%s", "", lines[i])
		fmt.Fprintf(logo, "\n")
	}

	version := fmt.Sprintf("\t v%s", utils.Version)
	fmt.Fprint(logo, version)
	return logo
}

func balanceView(balance *lnrpc.WalletBalanceResponse) string {
	if balance == nil {
		return fmt.Sprintf("Balance: [%s:-:b]%s\n", tcell.ColorGreen, DefaultBalanceView)
	}
	strBalance := fmt.Sprintf("Balance: [%s:-:b]%s \n", tcell.ColorGreen, shared.FormatAmountView(chainutil.Amount(balance.ConfirmedBalance), 6))
	strBalance += fmt.Sprintf("[-:-:-]Unconfirmed: [%s:-:b]%s\n", tcell.ColorGreen, shared.FormatAmountView(chainutil.Amount(balance.UnconfirmedBalance), 6))
	// strBalance += fmt.Sprintf("[-:-:-]Locked: [%s:-:b]%s", tcell.ColorGreen, shared.FormatAmountView(chainutil.Amount(balance.LockedBalance), 6))
	return strBalance
}
