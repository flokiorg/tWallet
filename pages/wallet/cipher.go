// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package wallet

import (
	"errors"
	"fmt"
	"time"

	"github.com/rivo/tview"

	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/shared"
	"github.com/gdamore/tcell/v2"
)

func (w *Wallet) showCipherCard() {

	form := tview.NewForm().
		AddPasswordField("Passphrase:", "", 0, '*', nil).
		AddButton("Cancel", w.closeModal).
		AddButton("OK", func() {

			// w.load.Wallet.RestoreByHex()

			closeButton := tview.NewFlex().
				AddItem(nil, 0, 1, false).
				AddItem(components.NewConfirmButton(w.nav.Application, "Close", true, tcell.ColorDefault, 1, w.nav.CloseModal), 0, 1, true).
				AddItem(nil, 0, 1, false)

			cipherCard, height, _ := components.NewCipher(w.load, []string{
				"unsupported",
			}, "unsupported")

			grid := tview.NewGrid().
				SetRows(0, height, 1, 0).
				SetColumns(0, 50, 0).
				SetBorders(false).
				AddItem(cipherCard, 1, 1, 1, 1, 0, 0, false).
				AddItem(closeButton, 2, 1, 1, 1, 0, 0, true)

			w.nav.ShowModal(grid) // 33=>26 12=>19 24=>23
		})

	lockerView := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().SetText(" Please enter your spending passphrase. "), 1, 1, false).
		AddItem(form, 0, 1, true)

	lockerView.SetTitle("🔒 Restricted Access").SetTitleColor(tcell.ColorGray).SetBackgroundColor(tcell.ColorOrange).SetBorder(true).SetBorderPadding(1, 1, 1, 1)

	w.nav.ShowModal(components.NewModal(lockerView, 50, 10))

}

func (w *Wallet) showChangePasswordForm() {

	w.load.Notif.CancelToast()

	info := tview.NewTextView()
	info.SetBackgroundColor(tcell.ColorDefault).SetBorderPadding(1, 1, 2, 2)
	info.SetText("\nYour wallet is password protected and encrypted.\nUse this dialog to change your password.")

	form := tview.NewForm()
	form.SetBorderPadding(1, 1, 2, 3).SetBackgroundColor(tcell.ColorDefault)
	form.AddPasswordField("Current passphrase:", "", 0, '*', nil).
		AddPasswordField("New passphrase:", "", 0, '*', nil).
		AddPasswordField("Confirm passphrase:", "", 0, '*', nil).
		AddButton("Cancel", w.closeModal).
		AddButton("OK", func() {
			w.load.Notif.CancelToast()

			oldPass := form.GetFormItem(0).(*tview.InputField)
			pass := form.GetFormItem(1).(*tview.InputField)
			passConf := form.GetFormItem(2).(*tview.InputField)

			if err := w.validateChangePasswordFields(pass.GetText(), passConf.GetText()); err != nil {
				w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]error:[-:-:-] %s", err.Error()), time.Second*30)
				w.load.Application.SetFocus(oldPass)
				return
			}
			if err := w.load.Wallet.ChangePrivatePassphrase([]byte(oldPass.GetText()), []byte(pass.GetText())); err != nil {
				w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]error:[-:-:-] %s", err.Error()), time.Second*30)
				w.load.Application.SetFocus(oldPass)
				return
			}
			w.load.Notif.ShowToastWithTimeout("✅ Password changed", time.Second*2)
			w.nav.CloseModal()
		})

	view := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(info, 6, 1, false).
		AddItem(form, 0, 1, true)

	view.SetTitle("🔒 Change Password").
		SetTitleColor(tcell.ColorGray).
		SetBackgroundColor(tcell.ColorOrange).
		SetBorder(true)

	w.nav.ShowModal(components.NewModal(view, 50, 18))

}

func (w *Wallet) validateChangePasswordFields(pass, passConf string) error {
	if len(pass) < shared.MinPasswordLength {
		return fmt.Errorf("New password must be at least %d characters!", shared.MinPasswordLength)
	}

	if pass != passConf {
		return errors.New("Passwords do not match!")
	}

	return nil
}
