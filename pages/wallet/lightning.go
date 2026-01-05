// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package wallet

import (
	"fmt"
	"strings"
	"time"

	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/flnd"
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
		if err == flnd.ErrDaemonNotRunning {
			w.load.Notif.ShowToast("[red:-:-]Wallet not running")
		} else {
			w.load.Notif.ShowToast(fmt.Sprintf("[red:-:-]Error: %v", err))
		}
		return
	}

	// Force opaque black
	bgColor := tcell.ColorDefault

	createBaseForm := func() *tview.Form {
		f := tview.NewForm()
		f.SetBackgroundColor(bgColor)
		f.SetBorder(false)
		f.SetBorderPadding(0, 0, 0, 0)
		f.SetButtonTextColor(tview.Styles.PrimaryTextColor)
		return f
	}

	addField := func(f *tview.Form, label, value string) {
		f.AddInputField(label, value, 0, nil, nil)
		item := f.GetFormItem(f.GetFormItemCount() - 1).(*tview.InputField)
		item.SetFieldBackgroundColor(tview.Styles.ContrastBackgroundColor)
		item.SetFieldTextColor(tview.Styles.PrimaryTextColor)
		item.SetLabelColor(tview.Styles.SecondaryTextColor)
		item.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			return nil
		})
	}

	formHub := createBaseForm()
	addField(formHub, "RPC Address", cfg.RpcAddress)
	addField(formHub, "Macaroon Hex", cfg.MacaroonHex)
	addField(formHub, "TLS Cert Hex", cfg.TLSCertHex)

	formNode := createBaseForm()
	addField(formNode, "Peer Address", cfg.PeerAddress)
	addField(formNode, "Identity PubKey", cfg.PubKey)

	copyFunc := func() {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("RPC Address: %s\n", cfg.RpcAddress))
		sb.WriteString(fmt.Sprintf("Macaroon Hex: %s\n", cfg.MacaroonHex))
		sb.WriteString(fmt.Sprintf("TLS Cert Hex: %s\n", cfg.TLSCertHex))
		sb.WriteString("\n\n")
		sb.WriteString(fmt.Sprintf("Peer Address: %s\n", cfg.PeerAddress))
		sb.WriteString(fmt.Sprintf("Identity PubKey: %s\n", cfg.PubKey))

		if err := shared.ClipboardCopy(sb.String()); err != nil {
			w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Copy failed:[-:-:-] %v", err), 2*time.Second)
			return
		}
		w.load.Notif.ShowToastWithTimeout("📋 Configuration copied!", 2*time.Second)
	}

	cpyBtn := components.NewConfirmButton(w.nav.Application, "Copy Config", true, bgColor, 3, copyFunc)
	closeBtn := components.NewConfirmButton(w.nav.Application, "Close", true, bgColor, 3, w.closeModal)

	buttons := tview.NewFlex()
	buttons.Box = tview.NewBox().SetBackgroundColor(bgColor).SetBorderPadding(0, 0, 2, 2)
	buttons.AddItem(cpyBtn, 0, 1, false).
		AddItem(closeBtn, 0, 1, true)

	middleContainer := tview.NewFlex().SetDirection(tview.FlexRow)
	middleContainer.SetBackgroundColor(bgColor)

	middleContainer.AddItem(tview.NewTextView().SetBackgroundColor(bgColor), 2, 0, false)
	middleContainer.AddItem(formHub, 0, 1, false)
	middleContainer.AddItem(tview.NewTextView().SetBackgroundColor(bgColor), 1, 0, false)

	sep := tview.NewTextView().SetTextColor(tcell.ColorDarkGray).SetDynamicColors(true)
	sep.SetBackgroundColor(bgColor)
	sep.SetText(strings.Repeat("─", 100))
	middleContainer.AddItem(sep, 1, 0, false)

	middleContainer.AddItem(tview.NewTextView().SetBackgroundColor(bgColor), 1, 0, false)
	middleContainer.AddItem(formNode, 0, 1, false)

	middleContainer.AddItem(buttons, 5, 1, true)

	borderedContainer := tview.NewFlex().SetDirection(tview.FlexColumn)
	borderedContainer.SetTitle(" Lightning Connection Details ").
		SetTitleColor(tcell.ColorGray).
		SetBackgroundColor(tcell.ColorOrange).
		SetBorder(true)

	borderedContainer.AddItem(tview.NewTextView().SetBackgroundColor(bgColor), 2, 1, false)
	borderedContainer.AddItem(middleContainer, 0, 1, true)
	borderedContainer.AddItem(tview.NewTextView().SetBackgroundColor(bgColor), 2, 1, false)

	// Modal
	w.nav.ShowModal(components.NewModal(borderedContainer, 85, 22, w.closeModal))
}
