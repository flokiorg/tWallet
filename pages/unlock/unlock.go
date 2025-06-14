// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package unlock

import (
	"fmt"
	"time"

	"github.com/rivo/tview"

	"github.com/flokiorg/flnd/flnwallet"
	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/shared"
	"github.com/gdamore/tcell/v2"
)

type Unlock struct {
	*tview.Flex
	load *load.Load
	nav  *load.Navigator
}

func NewPage(l *load.Load, showForm bool) *Unlock {
	p := &Unlock{
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
	fmt.Fprintf(logo, "Tap [[%s:-:-]u[-:-:-]] to unlock", tcell.ColorLightSkyBlue)

	hFlex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(logo, 0, 1, true).
		AddItem(nil, 0, 1, false)

	vFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(hFlex, 19, 1, true).
		AddItem(nil, 0, 1, false)

	p.AddItem(vFlex, 0, 1, true)

	if showForm {
		go p.load.QueueUpdateDraw(func() {
			p.showUnlockForm()
		})
	}

	return p
}

func (p *Unlock) handleKeys(event *tcell.EventKey) *tcell.EventKey {

	if event.Key() != tcell.KeyRune {
		return event
	}

	switch event.Rune() {
	case 'u':
		p.showUnlockForm()
	}

	return event

}

func (p *Unlock) showUnlockForm() {

	p.load.Notif.CancelToast()

	var isBusy bool

	info := tview.NewTextView()
	info.SetBackgroundColor(tcell.ColorDefault).SetBorderPadding(1, 1, 2, 2)
	info.SetText("\nThis wallet is locked.\nEnter your passphrase to unlock it.")

	form := tview.NewForm()
	form.SetBorderPadding(1, 1, 2, 3).SetBackgroundColor(tcell.ColorDefault)
	form.AddPasswordField("Lock passphrase:", p.load.AppConfig.DefaultPassword, 0, '*', nil).
		AddButton("Unlock", func() {

			go func() {
				if isBusy {
					return
				}
				isBusy = true
				defer func() { isBusy = false }()

				passInput := form.GetFormItem(0).(*tview.InputField)
				p.load.Notif.CancelToast()
				p.load.Notif.ShowToast("🔒 unlocking...")

				if err := p.load.Wallet.Unlock(passInput.GetText()); err != nil {
					p.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*30)
					passInput.SetText(p.load.AppConfig.DefaultPassword)
					p.load.Application.SetFocus(passInput)
					return
				}

				sub := p.load.Wallet.Subscribe()
				defer p.load.Wallet.Unsubscribe(sub)
				for {
					select {
					case u := <-sub:
						switch u.State {

						case flnwallet.StatusDown:
							if u.Err != nil {
								p.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", u.Err.Error()), time.Second*30)
							}
							return

						case flnwallet.StatusReady, flnwallet.StatusSyncing:
							p.load.Notif.ShowToastWithTimeout("🔓 Unlocked", time.Second*2)
							p.load.QueueUpdateDraw(func() {
								p.load.Go(shared.WALLET)
							})
							return

						default:
							continue
						}

					case <-time.After(time.Second * 20):
						p.load.Notif.ShowToast("🔒 Unlock timed out. Try again.")
						return
					}
				}
			}()

		})

	view := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(info, 6, 1, false).
		AddItem(form, 0, 1, true)

	view.SetTitle("🔒 Locked").
		SetTitleColor(tcell.ColorGray).
		SetBackgroundColor(tcell.ColorOrange).
		SetBorder(true)

	p.nav.ShowModal(components.NewModal(view, 50, 15, p.nav.CloseModal))

}
