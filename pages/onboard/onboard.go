// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package onboard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rivo/tview"

	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/flnd"
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

	pages     *tview.Pages
	restoring bool
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

			dropdown := form.GetFormItem(0).(*tview.DropDown)
			seedField := form.GetFormItem(1).(*tview.TextArea)
			passField := form.GetFormItem(2).(*tview.InputField)
			confField := form.GetFormItem(3).(*tview.InputField)

			fromIndex, _ := dropdown.GetCurrentOption()
			seedText := seedField.GetText()
			pass := passField.GetText()
			passConf := confField.GetText()

			if err := p.validateFields(pass, passConf); err != nil {
				p.nav.ShowModal(components.ErrorModal(err.Error(), p.nav.CloseModal))
				return
			}

			p.showToast("⚡ restoring...")
			go p.restoreWallet(SeedType(fromIndex), seedText, pass)
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
			go p.createWallet(pass)
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

func (p *Onboard) restoreWallet(seedType SeedType, seedText, pass string) {
	var (
		words []string
		phex  string
		err   error
	)

	switch seedType {
	case HEX:
		phex = seedText
		words, err = p.load.Wallet.RestoreByEncipheredSeed(phex, pass)

	case MNEMONIC:
		words = extractSeedWords(seedText)
		phex, err = p.load.Wallet.RestoreByMnemonic(words, pass)

	default:
		err = fmt.Errorf("unexpected choice")
	}

	if err != nil {
		err = fmt.Errorf("failed to restore: %v", err)
	}

	p.load.QueueUpdateDraw(func() {
		if err != nil {
			p.pages.SwitchToPage(RestoreView)
			p.nav.ShowModal(components.ErrorModal(err.Error(), p.nav.CloseModal))
			p.restoring = false
			return
		}
		p.restoring = true
		if err := p.showCipherCard(phex, words); err != nil {
			p.pages.SwitchToPage(RestoreView)
			p.nav.ShowModal(components.ErrorModal(err.Error(), p.nav.CloseModal))
			p.restoring = false
		}
	})
}

func (p *Onboard) createWallet(pass string) {

	phex, words, err := p.load.Wallet.CreateWallet(pass)

	p.load.QueueUpdateDraw(func() {
		if err != nil {
			p.pages.SwitchToPage(NewWalletView)
			p.nav.ShowModal(components.ErrorModal(fmt.Sprintf("failed to create: %s", err.Error()), p.nav.CloseModal))
			return
		}
		p.restoring = false
		if err := p.showCipherCard(phex, words); err != nil {
			p.pages.SwitchToPage(NewWalletView)
			p.nav.ShowModal(components.ErrorModal(err.Error(), p.nav.CloseModal))
		}
	})
}

func (p *Onboard) buildCipherCard(phex string, words []string) (tview.Primitive, error) {

	confirmButton := components.NewConfirmButton(p.load.Application, "I have written down all words", true, tcell.ColorBlack, 3, func() {
		p.pages.HidePage(CipherView)
		cancel := func() {
			p.nav.CloseModal()
			p.pages.SwitchToPage(CipherView)
		}
		p.nav.ShowModal(components.NewDialog("confirm?", "Your mnemonic is NOT saved in the database and CANNOT be restored. Make sure to save it securely.", cancel, []string{"Cancel", "Risk Accepted"}, cancel, func() {
			p.nav.CloseModal()
			if p.restoring {
				go p.monitorRestoreRecovery()
			} else {
				go func() {
					p.load.QueueUpdateDraw(func() {
						p.load.Go(shared.WALLET)
					})
				}()
			}
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

func (p *Onboard) monitorRestoreRecovery() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	update := func(status *load.RecoveryStatus) bool {
		msg := fmt.Sprintf("⏳ Recovery in progress… [%d] UTXO recovered\n%.2f%% complete", status.UTXOCount, status.Info.Progress*100)
		p.load.QueueUpdateDraw(func() {
			p.showToast(msg)
		})
		return true
	}

	p.load.QueueUpdateDraw(func() {
		p.showToast("⌛ Waiting for wallet RPC to be ready…")
	})

	if err := p.waitForWalletRPC(ctx); err != nil {
		p.restoring = false
		p.load.QueueUpdateDraw(func() {
			msg := fmt.Sprintf("recovery failed: %v\nPress Ctrl+C to exit if stuck.", err)
			p.nav.ShowModal(components.ErrorModal(msg, p.nav.CloseModal))
		})
		return
	}

	status, err := p.load.MonitorRecovery(ctx, time.Second, update)
	p.restoring = false
	if err != nil {
		p.load.QueueUpdateDraw(func() {
			msg := fmt.Sprintf("recovery failed: %v\nPress Ctrl+C to exit if stuck.", err)
			p.nav.ShowModal(components.ErrorModal(msg, p.nav.CloseModal))
		})
		return
	}

	finalCount := 0
	if status != nil {
		finalCount = status.UTXOCount
	}

	p.load.QueueUpdateDraw(func() {
		p.showToast(fmt.Sprintf("✅ Recovery complete! [%d] UTXO recovered\n⌛ Waiting for wallet RPC to be ready…", finalCount))
	})

	if err := p.waitForWalletReady(ctx); err != nil {
		p.load.QueueUpdateDraw(func() {
			msg := fmt.Sprintf("wallet not ready after recovery: %v\nPress Ctrl+C to exit if stuck.", err)
			p.nav.ShowModal(components.ErrorModal(msg, p.nav.CloseModal))
		})
		return
	}

	p.load.QueueUpdateDraw(func() {
		p.load.Go(shared.WALLET)
	})
}

func (p *Onboard) waitForWalletReady(ctx context.Context) error {
	sub := p.load.Wallet.Subscribe()
	defer p.load.Wallet.Unsubscribe(sub)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update, ok := <-sub:
			if !ok || update == nil {
				return fmt.Errorf("wallet subscription closed while waiting for RPC ready")
			}
			if update.State == flnd.StatusReady {
				return nil
			}
			if update.State == flnd.StatusDown && update.Err != nil {
				return update.Err
			}
		}
	}
}

func (p *Onboard) waitForWalletRPC(ctx context.Context) error {
	sub := p.load.Wallet.Subscribe()
	defer p.load.Wallet.Unsubscribe(sub)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update, ok := <-sub:
			if !ok || update == nil {
				return fmt.Errorf("wallet subscription closed while waiting for RPC")
			}

			switch update.State {
			case flnd.StatusReady, flnd.StatusBlock, flnd.StatusTransaction, flnd.StatusSyncing:
				return nil
			case flnd.StatusDown:
				if update.Err != nil {
					return update.Err
				}
			}
		}
	}
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
