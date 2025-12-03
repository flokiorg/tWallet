// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package wallet

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/flokiorg/go-flokicoin/chaincfg"
	"github.com/flokiorg/go-flokicoin/chainutil"
	"github.com/flokiorg/go-flokicoin/crypto"
	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/shared"
)

func (w *Wallet) showMessageTools() {
	if w.load == nil || w.load.Wallet == nil {
		return
	}

	w.load.Notif.CancelToast()

	signForm := tview.NewForm()
	signForm.SetBackgroundColor(tcell.ColorDefault).
		SetBorderPadding(1, 2, 2, 2)

	signForm.AddTextArea("Message:", "", 0, 4, 0, nil).
		AddInputField("Signing address:", "", 0, nil, nil).
		AddTextArea("Signature:", "Not signed yet", 0, 5, 0, nil)

	verifyForm := tview.NewForm()
	verifyForm.SetBackgroundColor(tcell.ColorDefault).
		SetBorderPadding(1, 2, 2, 2)

	verifyForm.AddTextArea("Message:", "", 0, 4, 0, nil).
		AddInputField("Address:", "", 0, nil, nil).
		AddTextArea("Signature:", "", 0, 5, 0, nil).
		AddTextView("Status:", "[gray::]Not verified", 0, 1, true, false).
		AddTextView("Recovered pubkey:", "[gray::]-", 0, 1, true, false)

	var (
		signHandler     func()
		copySignatureFn func()
	)

	signForm.AddButton("Cancel", w.closeModal)
	signForm.AddButton("Sign", func() {
		if signHandler != nil {
			signHandler()
		}
	})
	signForm.AddButton("Copy signature", func() {
		if copySignatureFn != nil {
			copySignatureFn()
		}
	})

	var (
		verifyHandler func()
	)

	verifyForm.AddButton("Cancel", w.closeModal)
	verifyForm.AddButton("Verify", func() {
		if verifyHandler != nil {
			verifyHandler()
		}
	})

	signMsgField, _ := signForm.GetFormItem(0).(*tview.TextArea)
	signAddressField, _ := signForm.GetFormItem(1).(*tview.InputField)
	signOutputView, _ := signForm.GetFormItem(2).(*tview.TextArea)

	verifyMsgField, _ := verifyForm.GetFormItem(0).(*tview.TextArea)
	verifyAddressField, _ := verifyForm.GetFormItem(1).(*tview.InputField)
	verifySignatureField, _ := verifyForm.GetFormItem(2).(*tview.TextArea)
	verifyStatusView, _ := verifyForm.GetFormItem(3).(*tview.TextView)
	verifyPubKeyView, _ := verifyForm.GetFormItem(4).(*tview.TextView)

	var (
		signButton        *tview.Button
		copySignatureBtn  *tview.Button
		verifyButton      *tview.Button
		copyPubKeyButton  *tview.Button
		currentSignature  string
		currentRecovered  string
		lastSignedMessage string
		lastSignedAddress string
	)

	if idx := signForm.GetButtonIndex("Sign"); idx >= 0 {
		signButton = signForm.GetButton(idx)
	}
	if idx := signForm.GetButtonIndex("Copy signature"); idx >= 0 {
		copySignatureBtn = signForm.GetButton(idx)
		copySignatureBtn.SetDisabled(true)
	}
	if idx := verifyForm.GetButtonIndex("Verify"); idx >= 0 {
		verifyButton = verifyForm.GetButton(idx)
	}
	if idx := verifyForm.GetButtonIndex("Copy pubkey"); idx >= 0 {
		copyPubKeyButton = verifyForm.GetButton(idx)
		copyPubKeyButton.SetDisabled(true)
	}

	disableSignInputs := func(disabled bool) {
		if signMsgField != nil {
			signMsgField.SetDisabled(disabled)
		}
		if signAddressField != nil {
			signAddressField.SetDisabled(disabled)
		}
		if signButton != nil {
			signButton.SetDisabled(disabled)
		}
	}

	disableVerifyInputs := func(disabled bool) {
		if verifyMsgField != nil {
			verifyMsgField.SetDisabled(disabled)
		}
		if verifyAddressField != nil {
			verifyAddressField.SetDisabled(disabled)
		}
		if verifySignatureField != nil {
			verifySignatureField.SetDisabled(disabled)
		}
		if verifyButton != nil {
			verifyButton.SetDisabled(disabled)
		}
	}

	signHandler = func() {
		message := ""
		if signMsgField != nil {
			message = strings.TrimSpace(signMsgField.GetText())
		}
		address := ""
		if signAddressField != nil {
			address = strings.TrimSpace(signAddressField.GetText())
		}

		if message == "" {
			w.load.Notif.ShowToastWithTimeout("[red:-:-]Error:[-:-:-] message cannot be empty", time.Second*10)
			if signMsgField != nil {
				w.load.Application.SetFocus(signMsgField)
			}
			return
		}
		if address == "" {
			w.load.Notif.ShowToastWithTimeout("[red:-:-]Error:[-:-:-] address required", time.Second*10)
			if signAddressField != nil {
				w.load.Application.SetFocus(signAddressField)
			}
			return
		}

		disableSignInputs(true)
		if copySignatureBtn != nil {
			copySignatureBtn.SetDisabled(true)
		}
		if signButton != nil {
			signButton.SetLabel("Signing...")
		}
		if signOutputView != nil {
			signOutputView.SetText("Signing...", false)
		}
		w.load.Notif.ShowToast("✍️ signing message...")

		go func(msg, addr string) {
			signature, err := w.load.Wallet.SignMessage(addr, msg)
			w.load.Application.QueueUpdateDraw(func() {
				w.load.Notif.CancelToast()
				disableSignInputs(false)
				if signButton != nil {
					signButton.SetLabel("Sign")
				}

				if err != nil {
					currentSignature = ""
					if signOutputView != nil {
						signOutputView.SetText("Signing failed", false)
					}
					if copySignatureBtn != nil {
						copySignatureBtn.SetDisabled(true)
					}
					w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*20)
					return
				}

				currentSignature = signature
				lastSignedMessage = msg
				lastSignedAddress = addr
				if signOutputView != nil {
					signOutputView.SetText(signature, false)
				}
				if copySignatureBtn != nil {
					copySignatureBtn.SetDisabled(false)
				}
				if verifyMsgField != nil {
					verifyMsgField.SetText(msg, false)
				}
				if verifySignatureField != nil {
					verifySignatureField.SetText(signature, false)
				}
				if verifyAddressField != nil {
					verifyAddressField.SetText(addr)
				}
				shortAddr := shortAddress(addr)
				w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[green:-:-]Signature created for %s", shortAddr), time.Second*20)
			})
		}(message, address)
	}

	copySignatureFn = func() {
		if currentSignature == "" {
			return
		}
		if err := shared.ClipboardCopy(currentSignature); err != nil {
			w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*10)
			return
		}
		if verifySignatureField != nil {
			verifySignatureField.SetText(currentSignature, false)
		}
		w.load.Notif.ShowToastWithTimeout("📋 Signature copied", time.Second*10)
	}

	verifyHandler = func() {
		message := ""
		if verifyMsgField != nil {
			message = strings.TrimSpace(verifyMsgField.GetText())
		}
		signature := ""
		if verifySignatureField != nil {
			signature = strings.TrimSpace(verifySignatureField.GetText())
		}
		address := ""
		if verifyAddressField != nil {
			address = strings.TrimSpace(verifyAddressField.GetText())
		}
		if address == "" {
			if signAddressField != nil {
				address = strings.TrimSpace(signAddressField.GetText())
			}
		}
		if address == "" {
			address = lastSignedAddress
		}

		if message == "" {
			w.load.Notif.ShowToastWithTimeout("[red:-:-]Error:[-:-:-] message cannot be empty", time.Second*10)
			if verifyMsgField != nil {
				w.load.Application.SetFocus(verifyMsgField)
			}
			return
		}
		if signature == "" {
			w.load.Notif.ShowToastWithTimeout("[red:-:-]Error:[-:-:-] signature required", time.Second*10)
			if verifySignatureField != nil {
				w.load.Application.SetFocus(verifySignatureField)
			}
			return
		}
		if address == "" {
			w.load.Notif.ShowToastWithTimeout("[red:-:-]Error:[-:-:-] address required", time.Second*10)
			if verifyAddressField != nil {
				w.load.Application.SetFocus(verifyAddressField)
			}
			return
		}

		disableVerifyInputs(true)
		if verifyButton != nil {
			verifyButton.SetLabel("Verifying...")
		}
		if verifyStatusView != nil {
			verifyStatusView.SetText("[gray::]Verifying...")
		}
		if verifyPubKeyView != nil {
			verifyPubKeyView.SetText("[gray::]-")
		}
		if copyPubKeyButton != nil {
			copyPubKeyButton.SetDisabled(true)
		}
		w.load.Notif.ShowToast("🔍 verifying signature...")

		go func(msg, addr, sig string) {
			resp, err := w.load.Wallet.VerifyMessage(addr, msg, sig)
			w.load.Application.QueueUpdateDraw(func() {
				w.load.Notif.CancelToast()
				disableVerifyInputs(false)
				if verifyButton != nil {
					verifyButton.SetLabel("Verify")
				}

				if err != nil {
					currentRecovered = ""
					if verifyStatusView != nil {
						verifyStatusView.SetText("[red::-]Verification failed")
					}
					w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*20)
					return
				}

				if resp.GetValid() {
					if verifyStatusView != nil {
						verifyStatusView.SetText("[green:-:-]Signature valid")
					}
					w.load.Notif.ShowToastWithTimeout("[green:-:-]Signature verified", time.Second*12)
				} else {
					if verifyStatusView != nil {
						verifyStatusView.SetText("[red:-:-]Signature invalid")
					}
					w.load.Notif.ShowToastWithTimeout("[red:-:-]Invalid signature", time.Second*12)
					return
				}

				recoveredAddr := ""
				if addr != "" && w.load != nil && w.load.AppConfig != nil {
					if derived, derr := deriveAddressForVerification(resp.GetPubkey(), addr, w.load.AppConfig.Network); derr == nil {
						recoveredAddr = derived
					}
				}
				if recoveredAddr == "" {
					recoveredAddr = strings.TrimSpace(string(resp.GetPubkey()))
				}
				currentRecovered = recoveredAddr

				if verifyPubKeyView != nil {
					verifyPubKeyView.SetText(fmt.Sprintf("[gray::]%s", currentRecovered))
				}
				if copyPubKeyButton != nil {
					copyPubKeyButton.SetDisabled(false)
				}
			})
		}(message, address, signature)
	}

	pages := tview.NewPages()
	pages.AddPage("sign", signForm, true, true)
	pages.AddPage("verify", verifyForm, true, false)

	toggleRow := tview.NewFlex().SetDirection(tview.FlexColumn)
	toggleRow.SetBackgroundColor(tcell.ColorDefault)

	signBtn := tview.NewButton("Sign")
	verifyBtn := tview.NewButton("Verify")

	styleToggle := func(btn *tview.Button, active bool) {
		if active {
			btn.SetBackgroundColor(tcell.ColorWhite)
			btn.SetLabelColor(tcell.ColorBlack)
		} else {
			btn.SetBackgroundColor(tcell.ColorOrange)
			btn.SetLabelColor(tcell.ColorWhite)
		}
	}

	setMode := func(idx int) {
		if idx == 0 {
			pages.SwitchToPage("sign")
			styleToggle(signBtn, true)
			styleToggle(verifyBtn, false)
			if signMsgField != nil {
				w.load.Application.SetFocus(signMsgField)
			}
			return
		}

		pages.SwitchToPage("verify")
		styleToggle(signBtn, false)
		styleToggle(verifyBtn, true)
		if lastSignedMessage != "" && verifyMsgField != nil {
			verifyMsgField.SetText(lastSignedMessage, false)
		}
		if currentSignature != "" && verifySignatureField != nil {
			verifySignatureField.SetText(currentSignature, false)
		}
		if verifyAddressField != nil {
			addr := strings.TrimSpace(verifyAddressField.GetText())
			if addr == "" {
				switch {
				case lastSignedAddress != "":
					verifyAddressField.SetText(lastSignedAddress)
				case signAddressField != nil:
					verifyAddressField.SetText(signAddressField.GetText())
				}
			}
		}
		if verifyMsgField != nil {
			w.load.Application.SetFocus(verifyMsgField)
		}
	}

	signBtn.SetSelectedFunc(func() { setMode(0) })
	verifyBtn.SetSelectedFunc(func() { setMode(1) })

	toggleRow.AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorDefault), 2, 0, false).
		AddItem(signBtn, 0, 1, false).
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorDefault), 2, 0, false).
		AddItem(verifyBtn, 0, 1, false).
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorDefault), 4, 0, false)

	contentColumn := tview.NewFlex().SetDirection(tview.FlexColumn)
	contentColumn.SetBackgroundColor(tcell.ColorDefault)
	contentColumn.AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorDefault), 2, 0, false).
		AddItem(pages, 0, 1, true).
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorDefault), 2, 0, false)

	content := tview.NewFlex().SetDirection(tview.FlexColumn)
	content.SetBackgroundColor(tcell.ColorDefault)
	content.AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorDefault), 1, 0, false).
		AddItem(contentColumn, 0, 1, true).
		AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorDefault), 1, 0, false)

	container := tview.NewFlex().SetDirection(tview.FlexRow)
	container.SetTitle("Sign & Verify").
		SetTitleColor(tcell.ColorGray).
		SetBackgroundColor(tcell.ColorOrange).
		SetBorder(true)

	container.AddItem(tview.NewBox().SetBackgroundColor(tcell.ColorDefault), 2, 0, false).AddItem(toggleRow, 1, 0, false).
		AddItem(content, 0, 1, true)

	setMode(0)

	w.nav.ShowModal(components.NewModal(container, 74, 26, w.closeModal))
	if signMsgField != nil {
		w.load.Application.SetFocus(signMsgField)
	}
}

func deriveAddressForVerification(pub []byte, original string, params *chaincfg.Params) (string, error) {
	if params == nil {
		return "", fmt.Errorf("network parameters unavailable")
	}
	if len(pub) == 0 {
		return "", fmt.Errorf("empty public key")
	}
	addr, err := chainutil.DecodeAddress(original, params)
	if err != nil {
		return "", err
	}
	hash160 := chainutil.Hash160(pub)
	switch addr.(type) {
	case *chainutil.AddressWitnessPubKeyHash:
		newAddr, err := chainutil.NewAddressWitnessPubKeyHash(hash160, params)
		if err != nil {
			return "", err
		}
		return newAddr.EncodeAddress(), nil
	case *chainutil.AddressPubKeyHash:
		newAddr, err := chainutil.NewAddressPubKeyHash(hash160, params)
		if err != nil {
			return "", err
		}
		return newAddr.EncodeAddress(), nil
	case *chainutil.AddressTaproot:
		pk, err := crypto.ParsePubKey(pub)
		if err != nil {
			return "", err
		}
		xOnly := crypto.ToSerialized(pk).SchnorrSerialized()
		newAddr, err := chainutil.NewAddressTaproot(xOnly[:], params)
		if err != nil {
			return "", err
		}
		return newAddr.EncodeAddress(), nil
	default:
		if newAddr, err := chainutil.NewAddressWitnessPubKeyHash(hash160, params); err == nil {
			return newAddr.EncodeAddress(), nil
		}
		if newAddr, err := chainutil.NewAddressPubKeyHash(hash160, params); err == nil {
			return newAddr.EncodeAddress(), nil
		}
		return "", fmt.Errorf("unsupported address type")
	}
}
