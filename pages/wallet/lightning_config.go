// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package wallet

import (
	"fmt"
	"strings"
	"time"

	"github.com/flokiorg/flnd/flnwallet"
	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/shared"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (w *Wallet) showLightningConfigView() {
	if w.load == nil || w.load.Wallet == nil {
		return
	}

	w.load.Notif.CancelToast()

	cfg, err := w.load.Wallet.GetLightningConfig()
	if err != nil {
		if err == flnwallet.ErrDaemonNotRunning {
			w.load.Notif.ShowToast("[red:-:-]Wallet not running")
		} else {
			w.load.Notif.ShowToast(fmt.Sprintf("[red:-:-]Error: %v", err))
		}
		return
	}

	form := tview.NewForm()
	form.SetBackgroundColor(tcell.ColorDefault).
		SetBorderPadding(1, 2, 2, 2)

	addField := func(label, value string) {
		form.AddInputField(label, value, 0, nil, nil)

		item := form.GetFormItem(form.GetFormItemCount() - 1).(*tview.InputField)
		item.SetFieldBackgroundColor(tcell.ColorBlack)
		item.SetFieldTextColor(tcell.ColorWhite)
		item.SetLabelColor(tcell.ColorLightSkyBlue)
		item.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			return nil // Read-only
		})
	}

	addField("Address", cfg.Address)
	addField("Peer PubKey", cfg.PubKey)
	addField("Macaroon", cfg.MacaroonHex)
	addField("TLS Cert", cfg.TLSCertHex)

	form.AddButton("Copy Configuration", func() {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Address: %s\n", cfg.Address))
		sb.WriteString(fmt.Sprintf("PubKey: %s\n", cfg.PubKey))
		sb.WriteString(fmt.Sprintf("Macaroon: %s\n", cfg.MacaroonHex))
		sb.WriteString(fmt.Sprintf("TLS Cert: %s\n", cfg.TLSCertHex))

		if err := shared.ClipboardCopy(sb.String()); err != nil {
			w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Copy failed:[-:-:-] %v", err), 2*time.Second)
			return
		}
		w.load.Notif.ShowToastWithTimeout("📋 Configuration copied!", 2*time.Second)
	})

	form.AddButton("Close", w.closeModal)

	form.SetButtonTextColor(tcell.ColorWhite)

	container := tview.NewFlex().SetDirection(tview.FlexRow)
	container.SetTitle(" Lightning Node Configuration ").
		SetTitleColor(tcell.ColorOrange).
		SetBorderColor(tcell.ColorOrange).
		SetBackgroundColor(tcell.ColorDefault).
		SetBorder(true)

	container.AddItem(form, 0, 1, true)

	w.nav.ShowModal(components.NewModal(container, 100, 16, w.closeModal))
}
