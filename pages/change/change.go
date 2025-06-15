// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://wwc.opensource.org/licenses/mit-license.php.

package change

import (
	"errors"
	"fmt"
	"time"
	"unicode"

	"github.com/rivo/tview"

	"github.com/flokiorg/flnd/flnwallet"
	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/shared"
	"github.com/gdamore/tcell/v2"
)

type Change struct {
	*tview.Flex
	load *load.Load
	nav  *load.Navigator
}

func NewPage(l *load.Load) *Change {
	p := &Change{
		Flex: tview.NewFlex(),
		load: l,
		nav:  l.Nav,
	}

	netColor := shared.NetworkColor(*l.AppConfig.Network)

	p.SetBorder(true).
		SetTitleAlign(tview.AlignCenter).
		SetTitleColor(netColor).
		SetBorderColor(netColor)

	p.SetInputCapture(p.handleKeys)

	logo := tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter)
	logo.SetBorder(false)

	fmt.Fprintf(logo, "[%s:-:-]\n%s[-:-:-]\n", netColor, shared.LOCK_IMAGE)
	fmt.Fprintf(logo, "Tap [[%s:-:-]u[-:-:-]] to Change", tcell.ColorLightSkyBlue)

	hFlex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(logo, 0, 1, true).
		AddItem(nil, 0, 1, false)

	vFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(hFlex, 19, 1, true).
		AddItem(nil, 0, 1, false)

	p.AddItem(vFlex, 0, 1, true)

	go p.load.QueueUpdateDraw(func() {
		p.showChangeForm()
	})

	return p
}

func (p *Change) handleKeys(event *tcell.EventKey) *tcell.EventKey {

	if event.Key() != tcell.KeyRune {
		return event
	}

	switch unicode.ToLower(event.Rune()) {
	case 'u':
		p.showChangeForm()
	}

	return event
}

func (c *Change) closeModal() {
	c.load.Notif.CancelToast()
	c.nav.CloseModal()
}

func (c *Change) showChangeForm() {

	c.closeModal()

	info := tview.NewTextView()
	info.SetBackgroundColor(tcell.ColorDefault).SetBorderPadding(1, 1, 2, 2)
	info.SetText("\nYour wallet is password protected and encrypted.\nUse this dialog to change your password.")

	var isBusy bool

	form := tview.NewForm()
	form.SetBorderPadding(1, 1, 2, 3).SetBackgroundColor(tcell.ColorDefault)
	form.AddPasswordField("Current passphrase:", c.load.AppConfig.DefaultPassword, 0, '*', nil).
		AddPasswordField("New passphrase:", c.load.AppConfig.DefaultPassword, 0, '*', nil).
		AddPasswordField("Confirm passphrase:", c.load.AppConfig.DefaultPassword, 0, '*', nil).
		AddButton("Cancel", c.closeModal).
		AddButton("OK", func() {
			go func() {

				if isBusy {
					return
				}
				isBusy = true
				defer func() { isBusy = false }()

				c.load.Notif.CancelToast()
				c.load.Notif.ShowToast("🔒 updating...")

				oldPass := form.GetFormItem(0).(*tview.InputField)
				pass := form.GetFormItem(1).(*tview.InputField)
				passConf := form.GetFormItem(2).(*tview.InputField)

				if err := c.validateOldPasswordField(oldPass.GetText()); err != nil {
					c.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]error:[-:-:-] %s", err.Error()), time.Second*30)
					c.load.QueueUpdateDraw(func() { c.load.Application.SetFocus(oldPass) })
					return
				}

				if err := c.validateChangePasswordFields(pass.GetText(), passConf.GetText()); err != nil {
					c.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]error:[-:-:-] %s", err.Error()), time.Second*30)
					c.load.QueueUpdateDraw(func() { c.load.Application.SetFocus(oldPass) })
					return
				}
				if err := c.load.Wallet.ChangePassphrase(oldPass.GetText(), pass.GetText()); err != nil {
					c.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]error:[-:-:-] %s", err.Error()), time.Second*30)
					c.load.QueueUpdateDraw(func() { c.load.Application.SetFocus(oldPass) })
					return
				}
				sub := c.load.Wallet.Subscribe()
				defer c.load.Wallet.Unsubscribe(sub)
				for {
					select {
					case u := <-sub:
						switch u.State {

						case flnwallet.StatusDown:
							if u.Err != nil {
								c.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", u.Err.Error()), time.Second*30)
							}
							return

						case flnwallet.StatusReady, flnwallet.StatusSyncing:
							c.load.Notif.ShowToastWithTimeout("✅ Password changed", time.Second*2)
							c.load.QueueUpdateDraw(func() {
								c.load.Go(shared.WALLET)
							})
							return

						default:
							continue
						}

					case <-time.After(time.Second * 20):
						c.load.Notif.ShowToast("Timeout, try again.")
						return
					}
				}
			}()

		})

	view := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(info, 6, 1, false).
		AddItem(form, 0, 1, true)

	view.SetTitle("🔒 Change Password").
		SetTitleColor(tcell.ColorGray).
		SetBackgroundColor(tcell.ColorOrange).
		SetBorder(true)

	c.nav.ShowModal(components.NewModal(view, 50, 18, c.nav.CloseModal))

}

func (c *Change) validateOldPasswordField(oldPass string) error {
	if len(oldPass) < shared.MinPasswordLength {
		return fmt.Errorf("old password must be at least %d characters", shared.MinPasswordLength)
	}
	return nil
}

func (c *Change) validateChangePasswordFields(pass, passConf string) error {
	if len(pass) < shared.MinPasswordLength {
		return fmt.Errorf("new password must be at least %d characters", shared.MinPasswordLength)
	}

	if pass != passConf {
		return errors.New("passwords do not match")
	}

	return nil
}
