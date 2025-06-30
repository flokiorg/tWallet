// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package wallet

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/flokiorg/flnd/lnrpc"
	"github.com/flokiorg/go-flokicoin/chainutil"
	"github.com/rivo/tview"

	"github.com/flokiorg/flnd/flnwallet"
	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/shared"
	"github.com/flokiorg/twallet/utils"
	"github.com/gdamore/tcell/v2"
)

type feeOption struct {
	label  string
	amount chainutil.Amount
}

type sendViewModel struct {
	amount, totalCost, fee chainutil.Amount
	address                chainutil.Address
	isSending              bool
	lastErr                error
	lokiPerVbyte           uint64
}

type Wallet struct {
	*components.Table
	nav  *load.Navigator
	load *load.Load

	mu sync.Mutex

	svCache           *sendViewModel
	quit              chan struct{}
	notifSubscription <-chan *load.NotificationEvent
	busy              bool
}

func NewPage(l *load.Load) tview.Primitive {

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

	netColor := shared.NetworkColor(*l.AppConfig.Network)

	w := &Wallet{
		Table:   components.NewTable("Transactions", columns, netColor, flnwallet.MaxTransactionsLimit),
		nav:     l.Nav,
		load:    l,
		svCache: &sendViewModel{},
		quit:    make(chan struct{}),
	}

	w.SetBorder(true).
		SetTitleAlign(tview.AlignCenter).
		SetTitleColor(netColor).
		SetBorderColor(netColor)

	w.SetInputCapture(w.handleKeys)

	go w.listenNewTransactions()

	return w
}

func (w *Wallet) handleKeys(event *tcell.EventKey) *tcell.EventKey {

	if event.Key() != tcell.KeyRune || w.busy {
		return event
	}

	switch unicode.ToLower(event.Rune()) {
	case 's':
		w.showTransfertView()
	case 'r':
		w.showReceiveView()
	case 'c':
		w.changePassword()
	case 'l':
		w.lockWallet()
	}

	return event

}

func (w *Wallet) changePassword() {

	w.nav.ShowModal(components.NewDialog(
		"Confirm Action",
		"To change your password, the wallet must first be locked. Do you want to proceed?",
		w.nav.CloseModal,
		[]string{"Cancel", "Yes"},
		w.nav.CloseModal,
		func() {
			if w.busy {
				return
			}
			w.busy = true
			go func() {
				w.load.Notif.ShowToast("🔒 locking...")
				w.load.Wallet.Restart(context.Background())
				w.load.Application.QueueUpdateDraw(func() {
					w.load.Go(shared.CHANGE)
					w.busy = false
				})
			}()
		},
	))

}

func (w *Wallet) lockWallet() {

	w.nav.ShowModal(components.NewDialog(
		"Confirm Action",
		"Are you sure you want to lock the wallet?",
		w.nav.CloseModal,
		[]string{"Cancel", "Yes"},
		w.nav.CloseModal,
		func() {
			if w.busy {
				return
			}
			w.busy = true
			go func() {
				w.load.Notif.ShowToast("🔒 locking...")
				w.load.Wallet.Restart(context.Background())
				w.load.Application.QueueUpdateDraw(func() {
					w.load.Go(shared.LOCK)
					w.busy = false
				})
			}()
		},
	))
}

func (w *Wallet) showTransfertView() {

	w.load.Notif.CancelToast()

	form := tview.NewForm()
	form.SetBackgroundColor(tcell.ColorDefault).SetBorderPadding(2, 2, 3, 3)
	form.AddTextArea("Destination Address:", "", 0, 2, 0, func(text string) { w.transferAmountChanged(form) }).
		AddInputField("Amount:", "", 0, nil, func(text string) { w.transferAmountChanged(form) }).
		AddTextView("Fee:", fmt.Sprintf("[gray::]%d", 0), 0, 1, true, false).
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
				w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*30)
				w.load.Application.SetFocus(addressField)
				return
			}

			if w.svCache == nil || w.svCache.totalCost <= 0 {
				var errMsg string
				if w.svCache != nil && w.svCache.lastErr != nil {
					errMsg = w.svCache.lastErr.Error()
				} else {
					errMsg = fmt.Sprintf("invalid amount: total:%v", w.svCache.totalCost)
				}
				w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", errMsg), time.Second*30)
				w.load.Application.SetFocus(amountField)
				return
			}

			recap := tview.NewTextView().SetDynamicColors(true)
			recap.SetBorderPadding(1, 2, 2, 2)
			fmt.Fprintf(recap, "\n")
			fmt.Fprintf(recap, " Destination Address:\n [gray::]%s[-::]\n\n", addressField.GetText())
			fmt.Fprintf(recap, " Amount:\n [gray::]%s[-::]\n\n", shared.FormatAmountView(amount, 6))
			recap.SetBackgroundColor(tcell.ColorDefault)

			cForm := tview.NewForm()
			cForm.SetBackgroundColor(tcell.ColorDefault).SetBorderPadding(0, 2, 3, 3)

			cForm.AddTextView("Available balance:", fmt.Sprintf("[gray::]%s", w.currentStrBalance()), 0, 1, true, false).
				AddTextView("Fee:", fmt.Sprintf("[gray::]%s", shared.FormatAmountView(w.svCache.fee, 6)), 0, 1, true, false).
				AddTextView("Total cost:", totalCostField.GetText(false), 0, 1, true, false).
				AddTextView("Balance After send:", newBalanceField.GetText(false), 0, 1, true, false).
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

						txhash, err := w.load.Wallet.Transfer(w.svCache.address, w.svCache.amount, w.svCache.lokiPerVbyte)
						if err != nil {
							w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*30)
							return
						}
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

	address, err := w.load.Wallet.GetNextAddress(w.load.AppConfig.UnusedAddressType)
	if err != nil {
		w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*30)
		return
	}

	strAddress := address.String()

	w.load.Logger.Trace().Str("address", strAddress).Msg("address requested")

	qrtxt, err := shared.GenerateQRText(strAddress)
	if err != nil {
		w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*30)
		return
	}

	label := tview.NewTextView()
	label.SetDynamicColors(true).
		SetText(fmt.Sprintf("[gray::-]Address:[-:-:-] \n%s", strAddress))
	label.SetBackgroundColor(tcell.ColorDefault).SetBorderPadding(1, 2, 2, 2)

	qrText := tview.NewTextView().SetWrap(true).SetWordWrap(true)
	qrText.SetBackgroundColor(tcell.ColorDefault)
	qrText.SetText(qrtxt).
		SetTextAlign(tview.AlignCenter)

	cpyBtn := components.NewConfirmButton(w.nav.Application, "copy", true, tcell.ColorDefault, 3, func() {
		w.load.Notif.CancelToast()
		if err := shared.ClipboardCopy(strAddress); err != nil {
			w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*30)
		}
	})
	nextAddrBtn := components.NewConfirmButton(w.nav.Application, "Next Address", true, tcell.ColorDefault, 3, func() {
		w.load.Notif.CancelToast()
		address, err := w.load.Wallet.GetNextAddress(w.load.AppConfig.UsedAddressType)
		if err != nil {
			w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*30)
			return
		}
		strAddress = address.String()
		qrtxt, err = shared.GenerateQRText(strAddress)
		if err != nil {
			w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*30)
			return
		}
		go func() {
			w.load.Application.QueueUpdateDraw(func() {
				label.SetText(fmt.Sprintf("[gray::-]Address:[-:-:-] \n%s", strAddress))
				qrText.SetText(qrtxt)
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

	var expTaprootSize int
	if utils.IsTaprootAddressType(w.load.AppConfig.UnusedAddressType) {
		expTaprootSize = 2
	}

	view.AddItem(label, 5+expTaprootSize, 0, false).
		AddItem(qrText, 19+expTaprootSize, 1, false).
		AddItem(buttons, 5, 1, true)

	w.nav.ShowModal(components.NewModal(view, 50, 31+expTaprootSize+expTaprootSize, w.nav.CloseModal))
}

func (w *Wallet) validateTransferFields(strAddress string, strAmount string) (chainutil.Address, chainutil.Amount, error) {

	address, err := chainutil.DecodeAddress(strAddress, w.load.AppConfig.Network)
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

	return address, amount, nil
}

func (w *Wallet) currentStrBalance() string {
	balance := w.load.GetBalance()
	return shared.FormatAmountView(chainutil.Amount(balance), 6)
}

func (w *Wallet) transferAmountChanged(form *tview.Form) {
	if form.GetFormItemCount() < 6 {
		return
	}

	w.load.Notif.CancelToast()

	addressField := form.GetFormItem(0).(*tview.TextArea)
	amountField := form.GetFormItem(1).(*tview.InputField)
	feeField := form.GetFormItem(2).(*tview.TextView)
	totalCostField := form.GetFormItem(5).(*tview.TextView)
	newBalanceField := form.GetFormItem(6).(*tview.TextView)

	var err error
	var address chainutil.Address
	var baseAmount float64
	var amount chainutil.Amount

	defer func() {
		if err != nil {
			w.svCache.totalCost = 0
			w.svCache.lastErr = err
			feeField.SetText(fmt.Sprintf("[gray::]%.2f", 0.0))
			totalCostField.SetText(fmt.Sprintf("[gray::]%.2f", 0.0))
			newBalanceField.SetText(fmt.Sprintf("[gray::]%s", w.currentStrBalance()))
		}
	}()

	address, err = chainutil.DecodeAddress(addressField.GetText(), w.load.AppConfig.Network)
	if err != nil {
		return
	}

	baseAmount, err = strconv.ParseFloat(amountField.GetText(), 64)
	if err != nil {
		return
	}
	amount, err = chainutil.NewAmount(baseAmount)
	if err != nil {
		return
	}

	estmFee, err := w.load.Wallet.Fee(address, amount)
	if err != nil {
		w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*30)
		return
	}
	txFee := chainutil.Amount(estmFee.FeeSat)

	balance := w.load.GetBalance()

	totalcost := amount + txFee
	newBalance := chainutil.Amount(balance) - totalcost

	w.svCache.lokiPerVbyte = estmFee.SatPerVbyte
	w.svCache.totalCost = totalcost
	w.svCache.fee = txFee
	feeField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(txFee, 6)))
	totalCostField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(totalcost, 6)))
	newBalanceField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(newBalance, 6)))
}

func (w *Wallet) closeModal() {
	w.load.Notif.CancelToast()
	w.nav.CloseModal()
	w.load.Application.SetFocus(w.Table)
}

func (w *Wallet) fetchTransactionsRows() [][]string {

	txs, err := w.load.Wallet.FetchTransactions()
	if err != nil {
		w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*30)
		return nil
	}

	rows := [][]string{}
	for _, tx := range txs {

		row := []string{}
		row = append(row, timestampToLocalString(tx.TimeStamp))
		row = append(row, shortTxID(tx.TxHash))
		row = append(row, formatOutputAddresses(tx.OutputDetails))
		flcAmount := chainutil.Amount(tx.Amount)

		if flcAmount > 0 {
			row = append(row, fmt.Sprintf("[green:-:-]%s", shared.FormatAmountView(flcAmount, 6)))
		} else {
			row = append(row, fmt.Sprintf("[red:-:-]%s", shared.FormatAmountView(flcAmount, 6)))
		}
		row = append(row, strconv.FormatInt(int64(tx.NumConfirmations), 10))

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

		case <-w.quit:
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
	close(w.quit)
}

func timestampToLocalString(ts int64) string {
	t := time.Unix(ts, 0).Local()
	return t.Format("2006-01-02 15:04:05")
}

func shortTxID(txID string) string {
	if len(txID) < 10 {
		return txID // not enough characters to shorten
	}
	return txID[:5] + "_" + txID[len(txID)-5:]
}

func formatOutputAddresses(outputs []*lnrpc.OutputDetail) string {
	maxDisplay := 1
	total := len(outputs)

	// Extract up to 3 addresses
	var parts []string
	for i := 0; i < total && i < maxDisplay; i++ {
		parts = append(parts, outputs[i].Address)
	}

	result := strings.Join(parts, ", ")

	// Add "+N more" if there are more addresses
	if total > maxDisplay {
		result += fmt.Sprintf(",(+%d)", total-maxDisplay)
	}

	return result
}
