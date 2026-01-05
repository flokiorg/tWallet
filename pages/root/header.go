// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package root

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"

	"github.com/flokiorg/go-flokicoin/chainutil"
	"github.com/flokiorg/twallet/flnd"
	"github.com/flokiorg/twallet/load"
	. "github.com/flokiorg/twallet/shared"
	"github.com/flokiorg/twallet/utils"
	"github.com/gdamore/tcell/v2"
)

type Header struct {
	*tview.Flex
	logo              *tview.TextView
	shortcuts         *tview.Flex
	balance           *tview.TextView
	hotkeys           *tview.TextView
	walletInfo        *tview.Grid
	load              *load.Load
	destroy           chan struct{}
	dcancel           func()
	nsub              <-chan *load.NotificationEvent
	state             flnd.Status
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

	if ev := h.load.Wallet.GetLastEvent(); ev != nil && ev.State == flnd.StatusLocked {
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
	if h.state != flnd.StatusLocked {
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

	h.setWalletInfoVisible(h.state != flnd.StatusLocked)

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

	case evt.State == flnd.StatusReady, evt.State == flnd.StatusTransaction, evt.State == flnd.StatusBlock:
		h.refreshBalance()

	case evt.State == flnd.StatusSyncing:
		h.showBalanceStatus("Syncing...", tcell.ColorYellow)

	case evt.State == flnd.StatusDown:
		h.showBalanceStatus("Reconnecting...", tcell.ColorOrange)

	case evt.State == flnd.StatusNone:
		h.showBalanceStatus("Connecting...", tcell.ColorYellow)

	case evt.State == flnd.StatusNoWallet:
		h.showBalanceStatus("Wallet not found.", tcell.ColorRed)

	case evt.State == flnd.StatusLocked:
		h.showBalanceStatus("Wallet locked.", tcell.ColorOrange)

	default:
		h.showBalanceStatus("Loading balance...", tcell.ColorYellow)
	}
}

func (h *Header) refreshBalance() {
	if h.balance == nil {
		return
	}
	var confirmed, unconfirmed, locked chainutil.Amount
	balance, err := h.load.Wallet.Balance()
	if err != nil {
		h.load.Logger.Warn().Err(err).Msg("unable to fetch balance")
		confirmed, unconfirmed, locked = h.load.GetBalance()
	} else {
		confirmed, unconfirmed, locked = chainutil.Amount(balance.ConfirmedBalance), chainutil.Amount(balance.UnconfirmedBalance), chainutil.Amount(balance.LockedBalance)
	}

	h.updateBalance(confirmed, unconfirmed, locked)
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

func (h *Header) updateBalance(confirmed, unconfirmed, locked chainutil.Amount) {
	h.load.Logger.Debug().
		Int64("confirmed", int64(confirmed)).
		Int64("unconfirmed", int64(unconfirmed)).
		Int64("locked", int64(locked)).
		Msg("balance updated")
	h.load.Cache.SetBalance(confirmed, unconfirmed, locked)
	h.load.Application.QueueUpdateDraw(func() {
		h.status = ""
		h.balance.SetText(balanceView(confirmed, unconfirmed, locked))
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

func buildLogShortcutView() *tview.Flex {
	accent := tcell.ColorLightSkyBlue

	col1 := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	col1.SetBorder(false)

	fmt.Fprintf(col1, "\n[%s:-:-]<ctrl+t>[gray:-:-] Transactions\n", accent)
	fmt.Fprintf(col1, "[%s:-:-]<ctrl+a>[gray:-:-] Addresses\n", accent)
	fmt.Fprintf(col1, "[%s:-:-]<ctrl+s>[gray:-:-] Sign & Verify", accent)

	col2 := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	col2.SetBorder(false)

	fmt.Fprintf(col2, "\n[%s:-:-]<ctrl+x>[gray:-:-] Resync\n", accent)
	fmt.Fprintf(col2, "[%s:-:-]<ctrl+l>[gray:-:-] Logs\n", accent)
	fmt.Fprintf(col2, "[%s:-:-]<ctrl+n>[gray:-:-] Lightning Config", accent)

	shortcuts := tview.NewFlex().
		AddItem(col1, 0, 1, false).
		AddItem(col2, 0, 1, false)

	// Add padding if needed via BorderPadding on the Flex or columns?
	// Creating wrapper or setting padding on columns.
	// Existing code had: SetBorderPadding(0, 0, 1, 1).
	// I can apply padding to the Flex if possible, otherwise wrapper.
	// Flex doesn't have SetBorderPadding directly unless it has Border enabled?
	// tview.Box has SetBorderPadding. Flex embeds Box. So yes.
	shortcuts.SetBorder(false).SetBorderPadding(0, 0, 1, 1)

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

func balanceView(confirmedBalance, unconfirmedBalance, lockedBalance chainutil.Amount) string {

	strBalance := fmt.Sprintf("Balance: [%s:-:b]%s\n", tcell.ColorGreen, FormatAmountView(chainutil.Amount(confirmedBalance), 6))

	if unconfirmedBalance > 0 || lockedBalance == 0 {
		strBalance += fmt.Sprintf("[-:-:-]Unconfirmed: [%s:-:b]%s\n", tcell.ColorGreen, FormatAmountView(chainutil.Amount(unconfirmedBalance), 6))
	}
	if lockedBalance > 0 {
		strBalance += fmt.Sprintf("[-:-:-]Locked: [%s:-:b]%s\n", tcell.ColorGreen, FormatAmountView(chainutil.Amount(lockedBalance), 6))
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
