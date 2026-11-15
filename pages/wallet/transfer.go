package wallet

import (
	"fmt"
	"strconv"
	"time"

	"github.com/flokiorg/go-flokicoin/chainutil"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/flnd"
	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/shared"
	"github.com/flokiorg/twallet/utils"
)

const (
	DefaultLockExpirationSeconds = 5 * 60 // 5min
)

type sendViewModel struct {
	amount, totalCost, fee chainutil.Amount
	address                chainutil.Address
	balanceAfter           chainutil.Amount
	isSending              bool
	isPreparing            bool
	isReleasing            bool
	lastErr                error
	lokiPerVbyte           uint64
	finalTx                *chainutil.Tx
	locks                  []*flnd.OutputLock
	feeCalcID              uint64
}

func (w *Wallet) showTransfertView() {

	w.load.Notif.CancelToast()

	confirmedBalanceView := shared.FormatAmountView(w.confirmedBalance(), 6)

	form := tview.NewForm()
	form.SetBackgroundColor(tcell.ColorDefault).SetBorderPadding(2, 2, 3, 3)
	form.AddTextArea("Destination Address:", "", 0, 2, 0, func(text string) { w.transferAmountChanged(form) }).
		AddInputField("Amount:", "", 0, nil, func(text string) { w.transferAmountChanged(form) }).
		AddTextView("Fee:", fmt.Sprintf("[gray::]%d", 0), 0, 1, true, false).
		AddTextView("", "", 0, 1, true, false).
		AddTextView("Available balance:", fmt.Sprintf("[gray::]%s", confirmedBalanceView), 0, 1, true, false).
		AddTextView("Total cost:", fmt.Sprintf("[gray::]%.2f", 0.0), 0, 1, true, false).
		AddTextView("Balance After send:", fmt.Sprintf("[gray::]%s", confirmedBalanceView), 0, 1, true, false)

	var nextHandler func()
	var nextButton, cancelButton *tview.Button

	form.AddButton("Cancel", func() {
		w.closeModal()
	})
	form.AddButton("Next", func() {
		if nextHandler != nil {
			nextHandler()
		}
	})

	if idx := form.GetButtonIndex("Cancel"); idx >= 0 {
		cancelButton = form.GetButton(idx)
	}
	if idx := form.GetButtonIndex("Next"); idx >= 0 {
		nextButton = form.GetButton(idx)
	}

	disableInputs := func(disable bool) {
		if item, ok := form.GetFormItem(0).(*tview.TextArea); ok {
			item.SetDisabled(disable)
		}
		if item, ok := form.GetFormItem(1).(*tview.InputField); ok {
			item.SetDisabled(disable)
		}
		if nextButton != nil {
			nextButton.SetDisabled(disable)
		}
		if cancelButton != nil {
			cancelButton.SetDisabled(false)
		}
	}

	nextHandler = func() {
		w.load.Notif.CancelToast()

		addressField := form.GetFormItem(0).(*tview.TextArea)
		amountField := form.GetFormItem(1).(*tview.InputField)
		feeField := form.GetFormItem(2).(*tview.TextView)
		totalCostField := form.GetFormItem(5).(*tview.TextView)
		newBalanceField := form.GetFormItem(6).(*tview.TextView)

		address, amount, err := w.validateTransferFields(addressField.GetText(), amountField.GetText())
		if err != nil {
			w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*30)
			w.load.Application.SetFocus(addressField)
			return
		}

		w.mu.Lock()
		if w.svCache.isPreparing {
			w.mu.Unlock()
			return
		}
		w.svCache.isPreparing = true
		w.mu.Unlock()

		disableInputs(true)
		if nextButton != nil {
			nextButton.SetLabel("Please wait...")
		}
		w.load.Notif.ShowToast("⏳ preparing transaction...")

		go func(addr chainutil.Address, amt chainutil.Amount, dstAddress string) {
			err := w.prepareTransfer(addr, amt)

			w.load.Application.QueueUpdateDraw(func() {
				w.load.Notif.CancelToast()

				w.mu.Lock()
				w.svCache.isPreparing = false
				w.mu.Unlock()

				if err != nil {
					disableInputs(false)
					if nextButton != nil {
						nextButton.SetLabel("Next")
					}
					w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*30)
					w.load.Application.SetFocus(addressField)
					return
				}

				feeField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(w.svCache.fee, 6)))
				totalCostField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(w.svCache.totalCost, 6)))
				newBalanceField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(w.svCache.balanceAfter, 6)))
				if nextButton != nil {
					nextButton.SetLabel("Next")
				}

				w.showTransferConfirmation(dstAddress, amt, totalCostField.GetText(false), newBalanceField.GetText(false))
			})
		}(address, amount, addressField.GetText())
	}

	view := tview.NewFlex()
	view.SetTitle("Send").
		SetTitleColor(tcell.ColorGray).
		SetBackgroundColor(tcell.ColorOrange).
		SetBorder(true)

	view.AddItem(form, 0, 1, true)

	w.nav.ShowModal(components.NewModal(view, 50, 22, w.closeModal))
}

func (w *Wallet) prepareTransfer(address chainutil.Address, amount chainutil.Amount) error {
	w.mu.Lock()
	w.svCache.finalTx = nil
	w.svCache.locks = nil
	w.mu.Unlock()

	feeResp, err := w.load.Wallet.Fee(address, amount)
	if err != nil {
		return err
	}

	txFee := chainutil.Amount(feeResp.FeeSat)
	totalCost := amount + txFee
	newBalance := w.confirmedBalance() - totalCost

	entry := map[string]int64{
		address.String(): int64(amount),
	}

	funded, err := w.load.Wallet.FundPsbt(entry, feeResp.SatPerVbyte, DefaultLockExpirationSeconds)
	if err != nil {
		return err
	}

	finalTx, err := w.load.Wallet.FinalizePsbt(funded.Packet)
	if err != nil {
		if err := w.load.Wallet.ReleaseOutputs(funded.Locks); err != nil {
			w.load.Logger.Warn().Err(err).Msg("failed to release outputs after finalize failure")
		}
		w.load.BroadcastBalanceRefresh()
		return err
	}

	w.mu.Lock()
	w.svCache.address = address
	w.svCache.amount = amount
	w.svCache.fee = txFee
	w.svCache.totalCost = totalCost
	w.svCache.balanceAfter = newBalance
	w.svCache.lokiPerVbyte = feeResp.SatPerVbyte
	w.svCache.finalTx = finalTx
	w.svCache.locks = funded.Locks
	w.svCache.lastErr = nil
	w.mu.Unlock()

	w.load.BroadcastBalanceRefresh()

	return nil
}

func (w *Wallet) showTransferConfirmation(address string, amount chainutil.Amount, totalCostText, newBalanceText string) {
	recap := tview.NewTextView().SetDynamicColors(true)
	recap.SetBorderPadding(1, 2, 2, 2)
	fmt.Fprintf(recap, "\n")
	fmt.Fprintf(recap, " Destination Address:\n [gray::]%s[-::]\n\n", address)
	fmt.Fprintf(recap, " Amount:\n [gray::]%s[-::]\n\n", shared.FormatAmountView(amount, 6))
	recap.SetBackgroundColor(tcell.ColorDefault)

	cForm := tview.NewForm()
	cForm.SetBackgroundColor(tcell.ColorDefault).SetBorderPadding(0, 2, 3, 3)

	cForm.AddTextView("Available balance:", fmt.Sprintf("[gray::]%s", shared.FormatAmountView(w.confirmedBalance(), 6)), 0, 1, true, false).
		AddTextView("Fee:", fmt.Sprintf("[gray::]%s", shared.FormatAmountView(w.svCache.fee, 6)), 0, 1, true, false).
		AddTextView("Total cost:", totalCostText, 0, 1, true, false).
		AddTextView("Balance After send:", newBalanceText, 0, 1, true, false).
		AddButton("Cancel", w.closeModal).
		AddButton("Send", func() {
			sendIdx := cForm.GetButtonIndex("Send")

			var sendBtn *tview.Button
			if sendIdx >= 0 {
				sendBtn = cForm.GetButton(sendIdx)
			}

			w.mu.Lock()
			if w.svCache.isSending {
				w.mu.Unlock()
				return
			}
			tx := w.svCache.finalTx
			w.svCache.isSending = true
			w.mu.Unlock()

			if tx == nil {
				w.mu.Lock()
				w.svCache.isSending = false
				w.mu.Unlock()
				w.load.Notif.ShowToastWithTimeout("[red:-:-]Error:[-:-:-] transaction not ready", time.Second*30)
				if sendBtn != nil {
					sendBtn.SetDisabled(false)
					sendBtn.SetLabel("Send")
				}
				return
			}

			if sendBtn != nil {
				sendBtn.SetDisabled(true)
				sendBtn.SetLabel("Sending...")
			}

			go func(tx *chainutil.Tx) {
				w.load.Notif.ShowToastWithTimeout("⚡ publishing...", time.Second*60)

				err := w.load.Wallet.PublishTransaction(tx)
				hash := tx.Hash()
				var txHash string
				if hash != nil {
					txHash = hash.String()
				}
				if txHash == "" {
					txHash = "unknown"
				}

				w.load.Application.QueueUpdateDraw(func() {
					w.mu.Lock()
					w.svCache.isSending = false
					if err == nil {
						w.svCache = &sendViewModel{}
					}
					w.mu.Unlock()

					if err != nil {
						if sendBtn != nil {
							sendBtn.SetDisabled(false)
							sendBtn.SetLabel("Send")
						}
						w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*30)
						return
					}

					shortHash := txHash
					if txHash != "unknown" && len(txHash) > 10 {
						shortHash = fmt.Sprintf("%s_%s", txHash[:5], txHash[len(txHash)-5:])
					}

					w.load.Logger.Info().
						Str("tx_hash", txHash).
						Msg("Transaction published, waiting for confirmation")
					w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("✅ Transaction Sent! Waiting for confirmation… (%s)", shortHash), time.Second*60)
					w.load.Notif.BroadcastWalletUpdate(&load.NotificationEvent{State: flnd.StatusTransaction})
					w.nav.CloseModal()
				})
			}(tx)
		})

	cView := tview.NewFlex().SetDirection(tview.FlexRow)
	cView.SetTitle("Confirm Send").SetTitleColor(tcell.ColorGray).SetBackgroundColor(tcell.ColorOrange).SetBorder(true)

	cView.AddItem(recap, 9, 1, false).
		AddItem(cForm, 0, 1, true)

	w.nav.ShowModal(components.NewModal(cView, 50, 22, w.closeModal))
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
			return
		}
		shortAddr := strAddress
		if len(shortAddr) > 14 {
			shortAddr = fmt.Sprintf("%s...%s", strAddress[:6], strAddress[len(strAddress)-6:])
		}
		w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("📋 Copied %s", shortAddr), time.Second*10)
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

func (w *Wallet) confirmedBalance() chainutil.Amount {
	balance, _, _ := w.load.GetBalance()
	return balance
}

func (w *Wallet) transferAmountChanged(form *tview.Form) {
	if form.GetFormItemCount() < 6 {
		return
	}

	addressField, ok := form.GetFormItem(0).(*tview.TextArea)
	if !ok {
		return
	}
	amountField, ok := form.GetFormItem(1).(*tview.InputField)
	if !ok {
		return
	}
	feeField, ok := form.GetFormItem(2).(*tview.TextView)
	if !ok {
		return
	}
	totalCostField, ok := form.GetFormItem(5).(*tview.TextView)
	if !ok {
		return
	}
	newBalanceField, ok := form.GetFormItem(6).(*tview.TextView)
	if !ok {
		return
	}

	resetFields := func() {
		feeField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(0, 6)))
		totalCostField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(0, 6)))
		newBalanceField.SetText(fmt.Sprintf("[gray::]%s", w.confirmedBalance()))
	}

	address, err := chainutil.DecodeAddress(addressField.GetText(), w.load.AppConfig.Network)
	if err != nil {
		resetFields()
		return
	}

	baseAmount, err := strconv.ParseFloat(amountField.GetText(), 64)
	if err != nil {
		resetFields()
		return
	}
	amount, err := chainutil.NewAmount(baseAmount)
	if err != nil {
		resetFields()
		return
	}

	w.load.Notif.CancelToast()

	w.mu.Lock()
	w.svCache.address = address
	w.svCache.amount = amount
	w.svCache.finalTx = nil
	w.svCache.fee = 0
	w.svCache.totalCost = 0
	w.svCache.balanceAfter = 0
	w.svCache.feeCalcID++
	reqID := w.svCache.feeCalcID
	w.svCache.finalTx = nil
	w.mu.Unlock()

	placeholder := "[gray::]calculating..."
	feeField.SetText(placeholder)
	totalCostField.SetText(placeholder)
	newBalanceField.SetText(placeholder)

	go func(id uint64, addr chainutil.Address, amt chainutil.Amount) {
		feeResp, feeErr := w.load.Wallet.Fee(addr, amt)

		var (
			txFee      chainutil.Amount
			totalCost  chainutil.Amount
			newBalance chainutil.Amount
			lokiRate   uint64
		)

		if feeErr == nil {
			txFee = chainutil.Amount(feeResp.FeeSat)
			totalCost = amt + txFee
			newBalance = w.confirmedBalance() - totalCost
			lokiRate = feeResp.SatPerVbyte
		}

		w.load.Application.QueueUpdateDraw(func() {
			w.mu.Lock()
			if id != w.svCache.feeCalcID {
				w.mu.Unlock()
				return
			}
			if feeErr != nil {
				w.svCache.lastErr = feeErr
				w.svCache.fee = 0
				w.svCache.totalCost = 0
				w.svCache.balanceAfter = 0
				w.mu.Unlock()
				resetFields()
				w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", feeErr.Error()), time.Second*30)
				return
			}

			w.svCache.lastErr = nil
			w.svCache.fee = txFee
			w.svCache.totalCost = totalCost
			w.svCache.balanceAfter = newBalance
			w.svCache.lokiPerVbyte = lokiRate
			w.svCache.finalTx = nil
			w.mu.Unlock()

			feeField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(txFee, 6)))
			totalCostField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(totalCost, 6)))
			newBalanceField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(newBalance, 6)))
		})
	}(reqID, address, amount)
}

func (w *Wallet) closeModal() {
	w.load.Notif.CancelToast()
	w.releasePreparedOutputs()
	w.nav.CloseModal()
	w.focusActiveView()
}

func (w *Wallet) releasePreparedOutputs() {
	w.mu.Lock()
	if w.svCache == nil || len(w.svCache.locks) == 0 || w.svCache.isSending || w.svCache.isPreparing || w.svCache.isReleasing {
		w.mu.Unlock()
		return
	}
	w.svCache.isReleasing = true
	locks := w.svCache.locks
	w.mu.Unlock()

	go func() {
		if err := w.load.Wallet.ReleaseOutputs(locks); err != nil {
			w.load.Logger.Warn().Err(err).Msg("failed to release prepared outputs")

			w.mu.Lock()
			w.svCache.isReleasing = false
			w.mu.Unlock()

			w.load.Application.QueueUpdateDraw(func() {
				w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] failed to release outputs: %s", err.Error()), time.Second*15)
			})
			return
		}

		w.mu.Lock()
		w.svCache = &sendViewModel{}
		w.svCache.isReleasing = false
		w.mu.Unlock()

		w.load.Logger.Info().Msg("released prepared outputs after cancelling transfer")
		w.load.BroadcastBalanceRefresh()
	}()
}
