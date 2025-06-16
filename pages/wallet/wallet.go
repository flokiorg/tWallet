// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package wallet

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/flokiorg/go-flokicoin/chainutil"
	"github.com/rivo/tview"

	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/shared"
	"github.com/gdamore/tcell/v2"
	"github.com/skip2/go-qrcode"
)

type sendViewModel struct {
	amount, feePerByte chainutil.Amount
	address            chainutil.Address
	isSending          bool
	totalCost          float64
	lastErr            error
}

type Wallet struct {
	*components.Table
	nav  *load.Navigator
	load *load.Load

	mu sync.Mutex

	svCache           *sendViewModel
	destroy           chan struct{}
	notifSubscription <-chan *load.NotificationEvent
}

func NewPage(l *load.Load) *Wallet {

	columns := []components.Column{
		{
			Name:  "Timestamp",
			Align: tview.AlignLeft,
		}, {
			Name:  "Tx ID",
			Align: tview.AlignLeft,
		}, {
			Name:  "Address",
			Align: tview.AlignLeft,
		}, {
			Name:  "Amount",
			Align: tview.AlignRight,
		}, {
			Name:     "Confirmations",
			Align:    tview.AlignCenter,
			IsSorted: true,
			SortDir:  components.Ascending,
		},
	}

	w := &Wallet{
		Table:   components.NewTable("Transactions", columns),
		nav:     l.Nav,
		load:    l,
		svCache: &sendViewModel{},
	}

	w.SetBorder(true).
		SetTitleAlign(tview.AlignCenter).
		SetTitleColor(tcell.ColorOrange).
		SetBorderColor(tcell.ColorOrange)

	w.SetInputCapture(w.handleKeys)

	go w.listenNewTransactions()

	return w
}

func (w *Wallet) handleKeys(event *tcell.EventKey) *tcell.EventKey {

	if event.Key() != tcell.KeyRune {
		return event
	}

	switch event.Rune() {
	case 's':
		w.showTransfertView()
	case 'r':
		w.showReceiveView()
	case 'c':
		w.showChangePasswordForm()
		// case 'b':
		// 	w.showCipherCard()
	}

	return event

}

func (w *Wallet) showTransfertView() {

	w.load.Notif.CancelToast()

	feeOptionsTab := make([]string, 0, len(w.load.Fees))
	for _, opt := range w.load.Fees {
		feeOptionsTab = append(feeOptionsTab, opt.Label)
	}
	form := tview.NewForm()
	form.SetBackgroundColor(tcell.ColorDefault).SetBorderPadding(2, 2, 3, 3)
	form.AddTextArea("Destination Address:", "", 0, 2, 0, func(text string) { w.transferAmountChanged(form) }).
		AddInputField("Amount:", "", 0, nil, func(text string) { w.transferAmountChanged(form) }).
		AddDropDown("Fee:", feeOptionsTab, 1, func(option string, optionIndex int) { w.transferAmountChanged(form) }).
		AddTextView("", "", 0, 1, true, false).
		AddTextView("Available balance:", fmt.Sprintf("[gray::]%s", w.currentStrBalance()), 0, 1, true, false).
		AddTextView("Total cost:", fmt.Sprintf("[gray::]%.2f", 0.0), 0, 1, true, false).
		AddTextView("Balance After send:", fmt.Sprintf("[gray::]%s", w.currentStrBalance()), 0, 1, true, false).
		AddButton("Cancel", w.closeModal).
		AddButton("Next", func() {
			w.load.Notif.CancelToast()

			addressField := form.GetFormItem(0).(*tview.TextArea)
			amountField := form.GetFormItem(1).(*tview.InputField)
			totalCostField := form.GetFormItem(5).(*tview.TextView)
			newBalanceField := form.GetFormItem(6).(*tview.TextView)

			_, amount, err := w.validateTransferFields(addressField.GetText(), amountField.GetText())
			if err != nil {
				w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]error:[-:-:-] %s", err.Error()), time.Second*30)
				w.load.Application.SetFocus(addressField)
				return
			}

			if w.svCache == nil || w.svCache.totalCost <= 0 {
				var errMsg string
				if w.svCache != nil && w.svCache.lastErr != nil {
					errMsg = w.svCache.lastErr.Error()
				} else if !w.load.Wallet.IsSynced() {
					errMsg = "Electrum is disconnected"
				} else {
					errMsg = fmt.Sprintf("invalid amount: total:%v", w.svCache.totalCost)
				}
				w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]error:[-:-:-] %s", errMsg), time.Second*30)
				w.load.Application.SetFocus(amountField)
				return
			}

			recap := tview.NewTextView().SetDynamicColors(true)
			recap.SetBorderPadding(1, 2, 2, 2)
			fmt.Fprintf(recap, "\n")
			fmt.Fprintf(recap, " Destination Address:\n [gray::]%s[-::]\n\n", addressField.GetText())
			fmt.Fprintf(recap, " Amount:\n [gray::]%s[-::]\n\n", shared.FormatAmountView(amount, 6))
			recap.SetBackgroundColor(tcell.ColorDefault)

			privPassField := tview.NewInputField().SetLabel("Spending passphrase:").SetMaskCharacter('*')

			cForm := tview.NewForm()
			cForm.SetBackgroundColor(tcell.ColorDefault).SetBorderPadding(0, 2, 3, 3)

			cForm.AddTextView("Available balance:", fmt.Sprintf("[gray::]%s", w.currentStrBalance()), 0, 1, true, false).
				AddTextView("Total cost:", totalCostField.GetText(false), 0, 1, true, false).
				AddTextView("Balance After send:", newBalanceField.GetText(false), 0, 1, true, false).
				AddFormItem(privPassField).
				AddButton("Cancel", w.closeModal).
				AddButton("Send", func() {

					go func() {
						w.mu.Lock()
						if w.svCache.isSending {
							return
						}
						w.svCache.isSending = true
						w.mu.Unlock()
						defer func() {
							w.mu.Lock()
							defer w.mu.Unlock()
							w.svCache.isSending = false
						}()

						w.load.Notif.ShowToastWithTimeout("⚡ sending...", time.Second*60)

						privPass := privPassField.GetText()
						if len(privPass) <= shared.MinPasswordLength {
							w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]error:[-:-:-] %s", "Incorrect passphrase length"), time.Second*30)
							w.load.Application.SetFocus(privPassField)
							return
						}

						tx, err := w.load.Wallet.SimpleTransfer([]byte(privPass), w.svCache.address, w.svCache.amount, w.svCache.feePerByte)
						if err != nil {
							w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]error:[-:-:-] %s", err.Error()), time.Second*30)
							w.load.Application.SetFocus(privPassField)
							return
						}
						txhash := tx.TxHash().String()
						w.load.Logger.Info().
							Str("tx_hash", txhash).
							Msg("Transaction sent, waiting for confirmation")
						w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("✅ Transaction Sent! Waiting for confirmation… (%s_%s)", txhash[:5], txhash[len(txhash)-5:]), time.Second*60)
						w.load.Notif.BroadcastWalletUpdate(&load.NotificationEvent{})
						w.svCache = &sendViewModel{}
						w.nav.CloseModal()
					}()

				})

			cView := tview.NewFlex().SetDirection(tview.FlexRow)
			cView.SetTitle("Confirm Send").SetTitleColor(tcell.ColorGray).SetBackgroundColor(tcell.ColorOrange).SetBorder(true)

			cView.AddItem(recap, 9, 1, false).
				AddItem(cForm, 0, 1, true)

			w.nav.ShowModal(components.NewModal(cView, 50, 22, w.nav.CloseModal))

		})

	view := tview.NewFlex()
	view.SetTitle("Send").
		SetTitleColor(tcell.ColorGray).
		SetBackgroundColor(tcell.ColorOrange).
		SetBorder(true)

	view.AddItem(form, 0, 1, true)

	w.nav.ShowModal(components.NewModal(view, 50, 22, w.nav.CloseModal))
}

func (w *Wallet) showReceiveView() {

	w.load.Notif.CancelToast()

	address, err := w.load.Wallet.GetLastAddress()
	if err != nil {
		w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]error:[-:-:-] %s", err.Error()), time.Second*30)
		return
	}

	strAddress := address.String()

	w.load.Logger.Trace().Str("address", strAddress).Msg("address requested")

	qr, err := qrcode.New(strAddress, qrcode.Highest)
	if err != nil {
		w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]error:[-:-:-] %s", err.Error()), time.Second*30)
		return
	}

	label := tview.NewTextView()
	label.SetDynamicColors(true).
		SetText(fmt.Sprintf("[gray::-]Address:[-:-:-] \n%s", strAddress))
	label.SetBackgroundColor(tcell.ColorDefault).SetBorderPadding(1, 2, 2, 2)

	qrText := tview.NewTextView()
	qrText.SetBackgroundColor(tcell.ColorDefault)
	qrText.SetText(qr.ToSmallString(false)).
		SetTextAlign(tview.AlignCenter)

	cpyBtn := components.NewConfirmButton(w.nav.Application, "copy", true, tcell.ColorDefault, 3, func() {
		w.load.Notif.CancelToast()
		if err := shared.ClipboardCopy(strAddress); err != nil {
			w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]error:[-:-:-] %s", err.Error()), time.Second*30)
		}
	})
	nextAddrBtn := components.NewConfirmButton(w.nav.Application, "Next Address", true, tcell.ColorDefault, 3, func() {
		w.load.Notif.CancelToast()
		address, err := w.load.Wallet.GetNextAddress()
		if err != nil {
			w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]error:[-:-:-] %s", err.Error()), time.Second*30)
			return
		}
		strAddress = address.String()
		qr, err = qrcode.New(strAddress, qrcode.Highest)
		if err != nil {
			w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]error:[-:-:-] %s", err.Error()), time.Second*30)
			return
		}
		go func() {
			w.load.Application.QueueUpdateDraw(func() {
				label.SetText(fmt.Sprintf("[gray::-]Address:[-:-:-] \n%s", strAddress))
				qrText.SetText(qr.ToSmallString(false))
			})
		}()
	})

	buttons := tview.NewFlex()
	buttons.Box = tview.NewBox().SetBackgroundColor(tcell.ColorDefault).SetBorderPadding(0, 0, 2, 2)
	buttons.AddItem(cpyBtn, 0, 1, true).
		AddItem(nextAddrBtn, 0, 1, false)

	view := tview.NewFlex().SetDirection(tview.FlexRow)
	view.SetTitle("Receive").
		SetTitleColor(tcell.ColorGray).
		SetBackgroundColor(tcell.ColorOrange).
		SetBorder(true)

	view.AddItem(label, 5, 1, false).
		AddItem(qrText, 20, 1, false).
		AddItem(buttons, 5, 1, true)

	w.nav.ShowModal(components.NewModal(view, 50, 32, w.nav.CloseModal))
}

func (w *Wallet) validateTransferFields(strAddress string, strAmount string) (chainutil.Address, float64, error) {

	address, err := chainutil.DecodeAddress(strAddress, w.load.Params.Network)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid address")
	}

	amountNum, err := strconv.ParseFloat(strAmount, 64)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid amount")
	}

	if amountNum <= 0 {
		return nil, 0, fmt.Errorf("invalid amount")
	}

	amount, err := chainutil.NewAmount(amountNum)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid amount")
	}

	w.svCache.address = address
	w.svCache.amount = amount

	return address, amount.ToFLC(), nil
}

func (w *Wallet) currentStrBalance() string {
	return shared.FormatAmountView(w.load.Wallet.Balance(), 6)
}

func (w *Wallet) transferAmountChanged(form *tview.Form) {
	if form.GetFormItemCount() < 6 {
		return
	}

	w.load.Notif.CancelToast()

	addressField := form.GetFormItem(0).(*tview.TextArea)
	amountField := form.GetFormItem(1).(*tview.InputField)
	feeField := form.GetFormItem(2).(*tview.DropDown)
	totalCostField := form.GetFormItem(5).(*tview.TextView)
	newBalanceField := form.GetFormItem(6).(*tview.TextView)

	var err error
	var address chainutil.Address
	var amount float64
	var txFee *chainutil.Amount

	defer func() {
		if err != nil {
			w.svCache.totalCost = 0
			w.svCache.lastErr = err
			totalCostField.SetText(fmt.Sprintf("[gray::]%.2f", w.svCache.totalCost))
			newBalanceField.SetText(fmt.Sprintf("[gray::]%s", w.currentStrBalance()))
		}
	}()

	address, err = chainutil.DecodeAddress(addressField.GetText(), w.load.Params.Network)
	if err != nil {
		return
	}

	amount, err = strconv.ParseFloat(amountField.GetText(), 64)
	if err != nil {
		return
	}

	feeOptionIndex, _ := feeField.GetCurrentOption() // feeCurrentIndex
	feeAmount := w.load.Fees[feeOptionIndex].Amount  // feeAmount

	txFee, err = w.load.Wallet.SimpleTransferFee(address, chainutil.Amount(amount), feeAmount)
	if err != nil {
		w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]error:[-:-:-] %s", err.Error()), time.Second*30)
		return
	}

	totalcost := amount + txFee.ToFLC()
	newBalance := w.load.Wallet.Balance() - totalcost

	if newBalance < 0 {
		err = errors.New("insufficient balance")
		w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]error:[-:-:-] %s", err.Error()), time.Second*30)
		return
	}

	w.svCache.feePerByte = feeAmount
	w.svCache.totalCost = totalcost
	totalCostField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(totalcost, 6)))
	newBalanceField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(newBalance, 6)))
}

func (w *Wallet) closeModal() {
	w.load.Notif.CancelToast()
	w.nav.CloseModal()
	w.load.Application.SetFocus(w.Table)
}

func (w *Wallet) fetchTransactionsRows() [][]string {

	if !w.load.Wallet.IsOpened() {
		return nil
	}

	result, err := w.load.Wallet.FetchTransactions()
	if err != nil {
		w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]error:[-:-:-] %s", err.Error()), time.Second*30)
		return nil
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Confirmations < result[j].Confirmations
	})

	rows := [][]string{}
	for _, tx := range result {
		row := []string{}
		row = append(row, tx.Timestamp)
		row = append(row, tx.TxID[:5]+"_"+tx.TxID[len(tx.TxID)-5:])
		row = append(row, tx.Address)
		if tx.Amount > 0 {
			row = append(row, fmt.Sprintf("[green:-:-]%s", shared.FormatAmountView(tx.Amount, 6)))
		} else {
			row = append(row, fmt.Sprintf("[red:-:-]%s", shared.FormatAmountView(tx.Amount, 6)))
		}
		row = append(row, strconv.FormatInt(tx.Confirmations, 10))

		rows = append(rows, row)
	}

	return rows

}

func (w *Wallet) listenNewTransactions() {

	w.notifSubscription = w.load.Notif.Subscribe()

	w.updateRows()

	for {
		select {
		case <-w.notifSubscription:
			w.updateRows()

		case <-w.destroy:
			return
		}
	}
}

func (w *Wallet) updateRows() {
	rows := w.fetchTransactionsRows()
	w.load.Application.QueueUpdateDraw(func() {
		w.Update(rows)
	})
}

func (w *Wallet) Destroy() {
	close(w.destroy)
}
