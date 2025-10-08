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
	. "github.com/flokiorg/twallet/shared"
	"github.com/flokiorg/twallet/utils"
	"github.com/gdamore/tcell/v2"
)

type Header struct {
	*tview.Flex
	logo      *tview.TextView
	shortcuts *tview.TextView
	// shortcutsWrap     *tview.Flex
	balance           *tview.TextView
	hotkeys           *tview.TextView
	walletInfo        *tview.Grid
	load              *load.Load
	destroy           chan struct{}
	dcancel           func()
	nsub              <-chan *load.NotificationEvent
	state             flnwallet.Status
	status            string
	walletInfoVisible bool
	shortcutsVisible  bool
}

func NewHeader(l *load.Load) *Header {
	h := &Header{
		Flex:    tview.NewFlex(),
		load:    l,
		destroy: make(chan struct{}),
	}

	h.logo = h.buildLogo()
	h.AddItem(h.logo, 30, 1, false)

	if ok, _ := l.Wallet.WalletExists(); !ok {
		return h
	}

	if ev := h.load.Wallet.GetLastEvent(); ev != nil && ev.State == flnwallet.StatusLocked {
		return h
	}

	h.balance = tview.NewTextView().
		SetDynamicColors(true).
		SetTextColor(tcell.ColorOrange).
		SetTextAlign(tview.AlignLeft)

	h.shortcuts = buildLogShortcutView()
	// h.shortcutsWrap = buildShortcutWrapper(h.shortcuts)

	statusMessage := ""
	statusColor := tcell.ColorYellow

	statusMessage = "Syncing..."
	statusColor = tcell.ColorYellow

	h.hotkeys = buildSendReceiveView()

	h.balance.SetText(balanceStatusView(statusMessage, statusColor))
	h.status = statusMessage

	walletInfo := tview.NewGrid().
		SetRows(1, 1, 1, 2).
		SetColumns(0)

	walletInfo.AddItem(h.balance, 1, 0, 2, 1, 0, 0, false).
		AddItem(h.hotkeys, 3, 0, 1, 1, 0, 0, false)

	h.walletInfo = walletInfo
	if h.state != flnwallet.StatusLocked {
		h.AddItem(h.shortcuts, 0, 1, false)
		h.shortcutsVisible = true
		h.AddItem(walletInfo, 30, 1, false)
		h.walletInfoVisible = true
	}

	h.nsub, h.dcancel = h.load.Notif.Subscribe()
	go h.updates()

	return h
}

func (h *Header) updates() {

	for {
		select {
		case evt, ok := <-h.nsub:
			if !ok {
				return
			}
			h.handleNotification(evt)

		case <-h.destroy:
			return
		}
	}
}

func (h *Header) handleNotification(evt *load.NotificationEvent) {
	if evt != nil {
		h.state = evt.State
	}

	h.setWalletInfoVisible(h.state != flnwallet.StatusLocked)

	if evt != nil {
		logEvent := h.load.Logger.Trace().
			Str("state", string(evt.State))
		if evt.BlockHeight > 0 {
			logEvent = logEvent.Uint32("block_height", evt.BlockHeight)
		}
		if evt.Err != nil {
			logEvent = logEvent.Err(evt.Err)
		}
		logEvent.Msg("header received notification")
	}

	switch {
	case evt == nil:
		h.refreshBalance()

	case evt.State == flnwallet.StatusReady, evt.State == flnwallet.StatusTransaction, evt.State == flnwallet.StatusBlock:
		h.refreshBalance()

	case evt.State == flnwallet.StatusSyncing:
		h.showBalanceStatus("Syncing...", tcell.ColorYellow)

	case evt.State == flnwallet.StatusDown:
		h.showBalanceStatus("Reconnecting...", tcell.ColorOrange)

	case evt.State == flnwallet.StatusNone:
		h.showBalanceStatus("Connecting...", tcell.ColorYellow)

	case evt.State == flnwallet.StatusNoWallet:
		h.showBalanceStatus("Wallet not found.", tcell.ColorRed)

	case evt.State == flnwallet.StatusLocked:
		h.showBalanceStatus("Wallet locked.", tcell.ColorOrange)

	default:
		h.showBalanceStatus("Loading balance...", tcell.ColorYellow)
	}
}

func (h *Header) refreshBalance() {
	if h.balance == nil {
		return
	}
	balance, err := h.load.Wallet.Balance()
	if err != nil {
		h.load.Logger.Warn().Err(err).Msg("unable to fetch balance")
		h.showBalanceStatus("Balance unavailable.", tcell.ColorRed)
		return
	}

	h.updateBalance(balance)
}

func (h *Header) showBalanceStatus(message string, color tcell.Color) {
	if h.balance == nil {
		return
	}
	h.load.Application.QueueUpdateDraw(func() {
		if h.status == message {
			return
		}
		h.status = message
		h.balance.SetText(balanceStatusView(message, color))
	})
}

func (h *Header) Destroy() {
	if h.dcancel != nil {
		h.dcancel()
	}
	select {
	case <-h.destroy:
		return
	default:
		close(h.destroy)
	}
}

func (h *Header) updateBalance(resp *lnrpc.WalletBalanceResponse) {
	h.load.Logger.Debug().
		Int64("confirmed", resp.ConfirmedBalance).
		Int64("unconfirmed", resp.UnconfirmedBalance).
		Msg("balance updated")
	h.load.Cache.SetBalance(resp)
	h.load.Application.QueueUpdateDraw(func() {
		h.status = ""
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

func buildLogShortcutView() *tview.TextView {
	accent := tcell.ColorLightSkyBlue
	shortcuts := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	shortcuts.SetBorder(false).
		SetBorderPadding(0, 0, 1, 1)

	fmt.Fprintf(shortcuts, "\n[%s:-:-]<ctrl+t>[gray:-:-] Transactions\n", accent)
	fmt.Fprintf(shortcuts, "[%s:-:-]<ctrl+l>[gray:-:-] Logs", accent)

	return shortcuts
}

func buildSendReceiveView() *tview.TextView {
	accent := tcell.ColorLightSkyBlue
	hotkeys := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	hotkeys.SetBorderPadding(0, 0, 0, 1)
	hotkeys.SetWrap(false).SetWordWrap(false)

	fmt.Fprintf(hotkeys, "\n[%s:-:b]<s>[-:-:-] Send  ", accent)
	fmt.Fprintf(hotkeys, "[%s:-:b]<r>[-:-:-] Receive", accent)

	return hotkeys
}

func buildShortcutWrapper(content tview.Primitive) *tview.Flex {
	left := tview.NewBox()
	right := tview.NewBox()
	wrap := tview.NewFlex()
	wrap.AddItem(left, 0, 1, false).
		AddItem(content, 0, 2, false).
		AddItem(right, 0, 1, false)
	return wrap
}

func balanceView(balance *lnrpc.WalletBalanceResponse) string {
	if balance == nil {
		return fmt.Sprintf("Balance: [%s:-:b]%s\n", tcell.ColorGreen, DefaultBalanceView)
	}
	strBalance := fmt.Sprintf("Balance: [%s:-:b]%s\n", tcell.ColorGreen, FormatAmountView(chainutil.Amount(balance.ConfirmedBalance), 6))

	locked := balance.LockedBalance
	unconfirmed := balance.UnconfirmedBalance

	if locked > 0 && unconfirmed > 0 {
		total := locked + unconfirmed
		strBalance += fmt.Sprintf("[-:-:-]Pending: [%s:-:b]%s\n", tcell.ColorGreen, FormatAmountView(chainutil.Amount(total), 6))
		return strBalance
	}

	if unconfirmed > 0 || locked == 0 {
		strBalance += fmt.Sprintf("[-:-:-]Unconfirmed: [%s:-:b]%s\n", tcell.ColorGreen, FormatAmountView(chainutil.Amount(unconfirmed), 6))
	}
	if locked > 0 {
		strBalance += fmt.Sprintf("[-:-:-]Locked: [%s:-:b]%s\n", tcell.ColorGreen, FormatAmountView(chainutil.Amount(locked), 6))
	}

	return strBalance
}

func balanceStatusView(message string, color tcell.Color) string {
	if message == "" {
		message = "loading..."
	}
	status := fmt.Sprintf("Balance: [%s:-:b]%s\n", color, message)
	status += fmt.Sprintf("[-:-:-]Unconfirmed: [%s:-:b]%s\n", color, DefaultBalanceView)
	return status
}

func (h *Header) setWalletInfoVisible(visible bool) {
	if h.load == nil {
		return
	}
	h.load.Application.QueueUpdateDraw(func() {
		if visible {
			if !h.shortcutsVisible {
				h.AddItem(h.shortcuts, 0, 1, false)
				h.shortcutsVisible = true
			}
			if h.walletInfo != nil && !h.walletInfoVisible {
				h.AddItem(h.walletInfo, 30, 1, false)
				h.walletInfoVisible = true
			}
			return
		}
		if h.walletInfoVisible && h.walletInfo != nil {
			h.RemoveItem(h.walletInfo)
			h.walletInfoVisible = false
		}
		if h.shortcutsVisible {
			h.RemoveItem(h.shortcuts)
			h.shortcutsVisible = false
		}
	})
}
