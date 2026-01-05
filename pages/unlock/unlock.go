// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package unlock

import (
	"fmt"
	"time"
	"unicode"

	"github.com/rivo/tview"

	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/flnd"
	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/shared"
	"github.com/gdamore/tcell/v2"
)

const (
	unlockInstructions = "\nThis wallet is locked.\nEnter your passphrase to unlock it."
	unlockingMessage   = "\nUnlocking wallet...\nPlease wait."
	unlockedMessage    = "\nWallet unlocked!\nLoading..."
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

	switch unicode.ToLower(event.Rune()) {
	case 'u':
		p.showUnlockForm()
	}

	return event

}

func (p *Unlock) showUnlockForm() {

	p.load.Notif.CancelToast()

	info := tview.NewTextView()
	info.SetBackgroundColor(tcell.ColorDefault).SetBorderPadding(1, 1, 2, 2)
	info.SetText(unlockInstructions)

	form := tview.NewForm()
	form.SetBorderPadding(1, 1, 2, 3).SetBackgroundColor(tcell.ColorDefault)
	form.AddPasswordField("Lock passphrase:", p.load.AppConfig.DefaultPassword, 0, '*', nil)

	form.AddButton("Unlock", func() {

		unlockButton := form.GetButton(0)
		passInput := form.GetFormItem(0).(*tview.InputField)
		pass := passInput.GetText()

		p.load.Notif.CancelToast()
		p.load.Notif.ShowToast("🔒 unlocking...")

		info.SetText(unlockingMessage)
		unlockButton.SetLabel("Loading...")
		unlockButton.SetDisabled(true)

		go p.handleUnlock(pass, passInput, info, unlockButton)
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

func (p *Unlock) handleUnlock(pass string, passInput *tview.InputField, info *tview.TextView, unlockButton *tview.Button) {
	err := p.load.Wallet.Unlock(pass)
	if err != nil {
		p.load.QueueUpdateDraw(func() {
			p.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*30)
			passInput.SetText(p.load.AppConfig.DefaultPassword)
			info.SetText(unlockInstructions)
			unlockButton.SetLabel("Unlock")
			unlockButton.SetDisabled(false)
			p.load.Application.SetFocus(passInput)
		})
		return
	}

	sub := p.load.Wallet.Subscribe()
	defer p.load.Wallet.Unsubscribe(sub)

	timer := time.NewTimer(time.Second * 20)
	defer timer.Stop()

	for {
		select {
		case u, ok := <-sub:
			if !ok || u == nil {
				p.load.QueueUpdateDraw(func() {
					info.SetText(unlockInstructions)
					unlockButton.SetLabel("Unlock")
					unlockButton.SetDisabled(false)
					p.load.Notif.ShowToast("🔒 Unlock failed. Try again.")
					p.load.Application.SetFocus(passInput)
				})
				return
			}
			switch u.State {
			case flnd.StatusDown:
				event := u
				p.load.QueueUpdateDraw(func() {
					if event.Err != nil {
						p.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", event.Err.Error()), time.Second*30)
					}
					info.SetText(unlockInstructions)
					unlockButton.SetLabel("Unlock")
					unlockButton.SetDisabled(false)
					p.load.Application.SetFocus(passInput)
				})
				return

			case flnd.StatusReady, flnd.StatusSyncing, flnd.StatusUnlocked:
				p.load.QueueUpdateDraw(func() {
					p.load.Notif.ShowToastWithTimeout("🔓 Unlocked", time.Second*1)
					info.SetText(unlockedMessage)
					unlockButton.SetLabel("Unlock")
					unlockButton.SetDisabled(false)
					p.load.Go(shared.WALLET)
				})
				return

			default:
				continue
			}

		case <-timer.C:
			p.load.QueueUpdateDraw(func() {
				p.load.Notif.ShowToast("🔒 Unlock timed out. Try again.")
				info.SetText(unlockInstructions)
				unlockButton.SetLabel("Unlock")
				unlockButton.SetDisabled(false)
				p.load.Application.SetFocus(passInput)
			})
			return
		}
	}
}
