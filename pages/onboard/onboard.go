// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package onboard

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"

	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/shared"
	. "github.com/flokiorg/twallet/shared"
	"github.com/gdamore/tcell/v2"
)

const (
	NewWalletView string = "new"
	RestoreView   string = "restore"
	CipherView    string = "cipher"
	ToastView     string = "toast"
)

type Onboard struct {
	*tview.Flex
	load      *load.Load
	nav       *load.Navigator
	view      string
	switchBtn *components.Switch

	pages *tview.Pages
}

func NewPage(l *load.Load) *Onboard {
	p := &Onboard{
		Flex:  tview.NewFlex(),
		load:  l,
		nav:   l.Nav,
		view:  NewWalletView,
		pages: tview.NewPages(),
	}

	netColor := NetworkColor(*l.AppConfig.Network)

	p.SetBorder(true).
		SetTitleAlign(tview.AlignCenter).
		SetTitleColor(netColor).
		SetBorderColor(netColor)

	p.switchBtn = components.NewSwitch(p.nav, "New Wallet", "Restore wallet", 0, func(index int) {
		switch index {
		case 0:
			p.pages.SwitchToPage(NewWalletView)
		case 1:
			p.pages.SwitchToPage(RestoreView)
		}
	})

	p.pages = tview.NewPages().
		AddPage(NewWalletView, p.buildNewWalletForm(), true, false).
		AddPage(RestoreView, p.buildRestoreForm(), true, false)

	p.AddItem(p.pages, 0, 1, true)
	return p
}

func (p *Onboard) showToast(text string) {
	p.pages.RemovePage(ToastView).AddAndSwitchToPage(ToastView, components.Toast(text), true)
}

func (p *Onboard) showCipherCard(phex string, words []string) error {
	view, err := p.buildCipherCard(phex, words)
	if err != nil {
		return err
	}
	p.pages.RemovePage(CipherView).AddAndSwitchToPage(CipherView, view, true)
	return nil
}

func (p *Onboard) buildRestoreForm() tview.Primitive {

	form := tview.NewForm()
	form.AddDropDown("From: ", []string{" Mnemonic ", " Hex "}, 0, func(label string, i int) {
		if form.GetFormItemCount() == 0 {
			return
		}
		seedField := form.GetFormItem(1).(*tview.TextArea)
		switch strings.TrimSpace(strings.ToLower(label)) {
		case "mnemonic":
			seedField.SetLabel("Mnemonic: ")
		case "hex":
			seedField.SetLabel("Hex: ")
		}
	}).
		AddTextArea("Mnemonic: ", "", 0, 0, 0, nil).
		AddPasswordField("Spending passphrase: ", p.load.AppConfig.DefaultPassword, 0, '*', nil).
		AddPasswordField("Confirm passphrase: ", p.load.AppConfig.DefaultPassword, 0, '*', nil).
		AddButton("Restore", func() {

			fromIndex, _ := form.GetFormItem(0).(*tview.DropDown).GetCurrentOption()
			seedText := form.GetFormItem(1).(*tview.TextArea).GetText()
			pass := form.GetFormItem(2).(*tview.InputField).GetText()
			passConf := form.GetFormItem(3).(*tview.InputField).GetText()

			if err := p.validateFields(pass, passConf); err != nil {
				p.nav.ShowModal(components.ErrorModal(err.Error(), p.nav.CloseModal))
				return
			}

			p.showToast("⚡ restoring...")
			go func() {

				var err error
				var phex string
				var words []string
				defer func() {
					if err != nil {
						p.load.QueueUpdateDraw(func() {
							p.pages.SwitchToPage(RestoreView)
							p.nav.ShowModal(components.ErrorModal(err.Error(), p.nav.CloseModal))
						})
					}
				}()

				st := SeedType(fromIndex)
				switch st {
				case HEX:
					phex = seedText
					words, err = p.load.Wallet.RestoreByEncipheredSeed(phex, pass)

				case MNEMONIC:
					words = extractSeedWords(seedText)
					phex, err = p.load.Wallet.RestoreByMnemonic(words, pass)

				default:
					err = fmt.Errorf("unexpected choise")
					return
				}

				if err != nil {
					err = fmt.Errorf("failed to restore: %v", err)
					return
				}

				p.load.QueueUpdateDraw(func() {
					if err := p.showCipherCard(phex, words); err != nil {
						p.pages.SwitchToPage(RestoreView)
						p.nav.ShowModal(components.ErrorModal(err.Error(), p.nav.CloseModal))
					}
				})
			}()

		})

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(p.switchBtn, 5, 0, false).
		AddItem(form, 17, 0, true).
		AddItem(tview.NewBox(), 0, 1, false)

	mainFlex := tview.NewFlex().
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(flex, 50, 0, true).
		AddItem(tview.NewBox(), 0, 1, false)

	return mainFlex
}

func (p *Onboard) buildNewWalletForm() tview.Primitive {

	form := tview.NewForm()
	form.AddPasswordField("Lock passphrase: ", p.load.AppConfig.DefaultPassword, 0, '*', nil).
		AddPasswordField("Confirm lock passphrase: ", p.load.AppConfig.DefaultPassword, 0, '*', nil).
		AddButton("Continue", func() {
			pass := form.GetFormItem(0).(*tview.InputField).GetText()
			passConf := form.GetFormItem(1).(*tview.InputField).GetText()

			if err := p.validateFields(pass, passConf); err != nil {
				p.nav.ShowModal(components.ErrorModal(err.Error(), p.nav.CloseModal))
				return
			}

			p.showToast("⚡ creating...")
			go func() {

				phex, words, err := p.load.Wallet.CreateWallet(pass)
				p.load.QueueUpdateDraw(func() {
					if err != nil {
						p.pages.SwitchToPage(NewWalletView)
						p.nav.ShowModal(components.ErrorModal(fmt.Sprintf("failed to create: %s", err.Error()), p.nav.CloseModal))
						return
					}
					if err := p.showCipherCard(phex, words); err != nil {
						p.pages.SwitchToPage(NewWalletView)
						p.nav.ShowModal(components.ErrorModal(err.Error(), p.nav.CloseModal))
					}
				})

			}()
		})

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(p.switchBtn, 5, 0, false).
		AddItem(form, 12, 0, true).
		AddItem(tview.NewBox(), 0, 1, false)

	mainFlex := tview.NewFlex().
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(flex, 50, 0, true).
		AddItem(tview.NewBox(), 0, 1, false)

	return mainFlex
}

func (p *Onboard) buildCipherCard(phex string, words []string) (tview.Primitive, error) {

	confirmButton := components.NewConfirmButton(p.load.Application, "I have written down all words", true, tcell.ColorBlack, 3, func() {
		p.pages.HidePage(CipherView)
		cancel := func() {
			p.nav.CloseModal()
			p.pages.SwitchToPage(CipherView)
		}
		p.nav.ShowModal(components.NewDialog("confirm?", "Your mnemonic is NOT saved in the database and CANNOT be restored. Make sure to save it securely.", cancel, []string{"Cancel", "Risk Accepted"}, cancel, func() {
			go func() {
				p.load.QueueUpdateDraw(func() {
					p.load.Go(shared.WALLET)
				})
			}()
		}))
	})
	cipherCard, height, err := components.NewCipher(p.load, words, phex)
	if err != nil {
		return nil, fmt.Errorf("cipher card error: %v", err)
	}

	// be sure to store your seed phrase backup in a secure location
	grid := tview.NewGrid().
		SetRows(0, height, 1, 3, 0).
		SetColumns(0, 50, 0).
		SetBorders(false).
		AddItem(cipherCard, 1, 1, 1, 1, 0, 0, true).
		AddItem(confirmButton, 3, 1, 1, 1, 0, 0, false)

	container := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(grid, height+5, 0, true).
		AddItem(tview.NewBox(), 0, 1, false)
	return container, nil
}

func (p *Onboard) validateFields(pass, passConf string) error {
	if pass != passConf {
		return fmt.Errorf("passwords do not match")
	}

	if len(pass) < shared.MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters", shared.MinPasswordLength)
	}

	return nil
}

func extractSeedWords(seed string) []string {
	seed = strings.TrimSpace(seed)
	return strings.Fields(seed)
}
