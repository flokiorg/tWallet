// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package wallet

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/flokiorg/flnd/flnwallet"
	"github.com/flokiorg/flnd/lnrpc"
	"github.com/flokiorg/go-flokicoin/chainutil"
	"github.com/rivo/tview"

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
	balanceAfter           chainutil.Amount
	isSending              bool
	isPreparing            bool
	isReleasing            bool
	lastErr                error
	lokiPerVbyte           uint64
	finalTx                *chainutil.Tx
	locks                  []*flnwallet.OutputLock
}

type walletView int

const (
	transactionsView walletView = iota
	logsView
)

const (
	transactionsPageName = "transactions"
	logsPageName         = "logs"
)

const (
	logPollInterval    = 750 * time.Millisecond
	maxInitialLogBytes = int64(2 * 1024 * 1024)
)

type Wallet struct {
	view     *tview.Pages
	table    *components.Table
	logView  *tview.TextView
	nav      *load.Navigator
	load     *load.Load
	viewMode walletView

	mu          sync.Mutex
	stateMu     sync.Mutex
	logMu       sync.Mutex
	placeholder string

	svCache  *sendViewModel
	quit     chan struct{}
	busy     bool
	nsub     <-chan *load.NotificationEvent
	cancelN  func()
	quitOnce sync.Once

	logLines   []string
	logQuit    chan struct{}
	logPath    string
	logReady   bool
	logMaxLine int
	logStatus  string
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

	table := components.NewTable("Transactions", columns, netColor, l.AppConfig.MaxTransactionsLimit)
	table.SetBorder(true).
		SetTitleAlign(tview.AlignCenter).
		SetTitleColor(netColor).
		SetBorderColor(netColor)

	logView := tview.NewTextView()
	logView.SetWrap(false).
		SetDynamicColors(false).
		SetScrollable(true).
		SetBorder(true).
		SetTitle(" Logs ").
		SetTitleAlign(tview.AlignCenter).
		SetTitleColor(netColor).
		SetBorderColor(netColor)
	logView.SetBorderPadding(1, 1, 2, 2)
	logView.SetChangedFunc(func() {
		logView.ScrollToEnd()
	})

	pages := tview.NewPages()
	pages.AddPage(transactionsPageName, table, true, true)
	pages.AddPage(logsPageName, logView, true, false)

	w := &Wallet{
		view:       pages,
		table:      table,
		logView:    logView,
		nav:        l.Nav,
		load:       l,
		svCache:    &sendViewModel{},
		quit:       make(chan struct{}),
		logQuit:    make(chan struct{}),
		viewMode:   transactionsView,
		logMaxLine: 2000,
	}

	w.view.SetInputCapture(w.handleKeys)

	w.applyPlaceholder("Loading transactions...")

	w.nsub, w.cancelN = l.Notif.Subscribe()
	go w.listenNewTransactions()
	go w.startLogTail()

	return w.view
}

func (w *Wallet) handleKeys(event *tcell.EventKey) *tcell.EventKey {

	if w.busy {
		return event
	}

	switch event.Key() {
	case tcell.KeyCtrlL:
		w.showLogsView()
		return nil
	case tcell.KeyCtrlT:
		w.showTransactionsView()
		return nil
	}

	if event.Key() != tcell.KeyRune {
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

func (w *Wallet) showLogsView() {
	if w.viewMode == logsView {
		return
	}
	w.view.SwitchToPage(logsPageName)
	w.viewMode = logsView
	w.focusActiveView()
}

func (w *Wallet) showTransactionsView() {
	if w.viewMode == transactionsView {
		return
	}
	w.view.SwitchToPage(transactionsPageName)
	w.viewMode = transactionsView
	w.focusActiveView()
}

func (w *Wallet) focusActiveView() {
	if w.load == nil || w.load.Application == nil {
		return
	}
	switch w.viewMode {
	case logsView:
		w.load.Application.SetFocus(w.logView)
	default:
		w.load.Application.SetFocus(w.table)
	}
}

func (w *Wallet) startLogTail() {
	if w.load == nil || w.load.AppConfig == nil || w.logView == nil || w.logQuit == nil {
		return
	}

	networkName := "unknown"
	if w.load.AppConfig.Network != nil {
		networkName = w.load.AppConfig.Network.Name
	}
	w.logPath = filepath.Join(w.load.AppConfig.Walletdir, "logs", "flokicoin", networkName, "flnd.log")
	w.setLogStatus(fmt.Sprintf("Loading log from %s", w.logPath))

	go w.tailLog()
}

func (w *Wallet) tailLog() {
	ticker := time.NewTicker(logPollInterval)
	defer ticker.Stop()

	var offset int64

	for {
		select {
		case <-w.logQuit:
			return
		case <-ticker.C:
		}

		info, err := os.Stat(w.logPath)
		if err != nil {
			if os.IsNotExist(err) {
				w.setLogStatus(fmt.Sprintf("Waiting for log file at %s", w.logPath))
			} else {
				w.setLogStatus(fmt.Sprintf("Log unavailable: %v", err))
			}
			continue
		}

		size := info.Size()
		if size < offset {
			offset = 0
		}

		f, err := os.Open(w.logPath)
		if err != nil {
			w.setLogStatus(fmt.Sprintf("Unable to open log file: %v", err))
			continue
		}

		if offset == 0 {
			start := int64(0)
			if size > maxInitialLogBytes {
				start = size - maxInitialLogBytes
			}
			if start > 0 {
				if _, err := f.Seek(start, io.SeekStart); err != nil {
					f.Close()
					offset = 0
					continue
				}
			}
			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				w.setLogStatus(fmt.Sprintf("Unable to read log file: %v", err))
				continue
			}
			lines := w.readLogLines(data)
			if start > 0 && len(lines) > 0 {
				lines = lines[1:]
			}
			if len(lines) > 0 {
				w.replaceLogLines(lines)
				w.logReady = true
			} else if !w.logReady {
				w.setLogStatus("Log file is empty.")
			}
			offset = size
			continue
		}

		if size == offset {
			f.Close()
			continue
		}

		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			offset = 0
			continue
		}

		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			w.setLogStatus(fmt.Sprintf("Unable to read log file: %v", err))
			continue
		}

		offset += int64(len(data))
		lines := w.readLogLines(data)
		if len(lines) > 0 {
			w.appendLogLines(lines)
			w.logReady = true
		}
	}
}

func (w *Wallet) replaceLogLines(lines []string) {
	if len(lines) == 0 {
		return
	}
	if w.logMaxLine > 0 && len(lines) > w.logMaxLine {
		lines = lines[len(lines)-w.logMaxLine:]
	}

	w.logMu.Lock()
	w.logLines = append([]string{}, lines...)
	w.logStatus = ""
	text := strings.Join(w.logLines, "\n")
	w.logMu.Unlock()

	w.updateLogView(text)
}

func (w *Wallet) appendLogLines(lines []string) {
	if len(lines) == 0 {
		return
	}

	w.logMu.Lock()
	w.logStatus = ""
	if w.logMaxLine > 0 {
		total := len(w.logLines) + len(lines)
		if total > w.logMaxLine {
			drop := total - w.logMaxLine
			if drop >= len(w.logLines) {
				w.logLines = append([]string{}, lines...)
			} else {
				w.logLines = append(append([]string{}, w.logLines[drop:]...), lines...)
			}
		} else {
			w.logLines = append(w.logLines, lines...)
		}
	} else {
		w.logLines = append(w.logLines, lines...)
	}
	text := strings.Join(w.logLines, "\n")
	w.logMu.Unlock()

	w.updateLogView(text)
}

func (w *Wallet) setLogStatus(message string) {
	w.logMu.Lock()
	if w.logStatus == message {
		w.logMu.Unlock()
		return
	}
	w.logStatus = message
	w.logReady = false
	w.logLines = []string{message}
	text := message
	w.logMu.Unlock()

	w.updateLogView(text)
}

func (w *Wallet) updateLogView(text string) {
	if w.load == nil || w.load.Application == nil {
		return
	}
	w.load.Application.QueueUpdateDraw(func() {
		if w.logView != nil {
			w.logView.SetText(text)
		}
	})
}

func (w *Wallet) readLogLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil && w.load != nil {
		w.load.Logger.Warn().Err(err).Msg("log scanner error")
	}
	return lines
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

			w.setFormDisabled(form, true)
			w.load.Notif.ShowToast("⏳ preparing transaction...")

			go func(addr chainutil.Address, amt chainutil.Amount, dstAddress string) {
				err := w.prepareTransfer(addr, amt)

				w.load.Application.QueueUpdateDraw(func() {
					w.load.Notif.CancelToast()

					w.mu.Lock()
					w.svCache.isPreparing = false
					w.mu.Unlock()

					if err != nil {
						w.setFormDisabled(form, false)
						w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*30)
						w.load.Application.SetFocus(addressField)
						return
					}

					feeField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(w.svCache.fee, 6)))
					totalCostField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(w.svCache.totalCost, 6)))
					newBalanceField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(w.svCache.balanceAfter, 6)))

					w.showTransferConfirmation(dstAddress, amt, totalCostField.GetText(false), newBalanceField.GetText(false))
				})
			}(address, amount, addressField.GetText())
		})

	view := tview.NewFlex()
	view.SetTitle("Send").
		SetTitleColor(tcell.ColorGray).
		SetBackgroundColor(tcell.ColorOrange).
		SetBorder(true)

	view.AddItem(form, 0, 1, true)

	w.nav.ShowModal(components.NewModal(view, 50, 22, w.closeModal))
}

func (w *Wallet) setFormDisabled(form *tview.Form, disabled bool) {
	if form == nil {
		return
	}
	for i := 0; i < form.GetFormItemCount(); i++ {
		form.GetFormItem(i).SetDisabled(disabled)
	}
	for i := 0; i < form.GetButtonCount(); i++ {
		form.GetButton(i).SetDisabled(disabled)
	}
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
	newBalance := chainutil.Amount(w.load.GetBalance()) - totalCost

	entry := map[string]int64{
		address.String(): int64(amount),
	}

	funded, err := w.load.Wallet.FundPsbt(entry, feeResp.SatPerVbyte)
	if err != nil {
		return err
	}

	finalTx, err := w.load.Wallet.FinalizePsbt(funded.Packet)
	if err != nil {
		if err := w.load.Wallet.ReleaseOutputs(funded.Locks); err != nil {
			w.load.Logger.Warn().Err(err).Msg("failed to release outputs after finalize failure")
		}
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

	cForm.AddTextView("Available balance:", fmt.Sprintf("[gray::]%s", w.currentStrBalance()), 0, 1, true, false).
		AddTextView("Fee:", fmt.Sprintf("[gray::]%s", shared.FormatAmountView(w.svCache.fee, 6)), 0, 1, true, false).
		AddTextView("Total cost:", totalCostText, 0, 1, true, false).
		AddTextView("Balance After send:", newBalanceText, 0, 1, true, false).
		AddButton("Cancel", w.closeModal).
		AddButton("Send", func() {
			sendIdx := cForm.GetButtonIndex("Send")
			cancelIdx := cForm.GetButtonIndex("Cancel")

			var sendBtn, cancelBtn *tview.Button
			if sendIdx >= 0 {
				sendBtn = cForm.GetButton(sendIdx)
			}
			if cancelIdx >= 0 {
				cancelBtn = cForm.GetButton(cancelIdx)
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
				}
				if cancelBtn != nil {
					cancelBtn.SetDisabled(false)
				}
				return
			}

			if sendBtn != nil {
				sendBtn.SetDisabled(true)
			}
			if cancelBtn != nil {
				cancelBtn.SetDisabled(true)
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
						}
						if cancelBtn != nil {
							cancelBtn.SetDisabled(false)
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
					w.load.Notif.BroadcastWalletUpdate(&load.NotificationEvent{State: flnwallet.StatusTransaction})
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
			w.mu.Lock()
			w.svCache.totalCost = 0
			w.svCache.balanceAfter = 0
			w.svCache.lastErr = err
			w.svCache.finalTx = nil
			w.mu.Unlock()
			feeField.SetText(fmt.Sprintf("[gray::]%.2f", 0.0))
			totalCostField.SetText(fmt.Sprintf("[gray::]%.2f", 0.0))
			newBalanceField.SetText(fmt.Sprintf("[gray::]%s", w.currentStrBalance()))
		} else {
			w.mu.Lock()
			w.svCache.lastErr = nil
			w.mu.Unlock()
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

	w.load.Notif.CancelToast()

	w.mu.Lock()
	w.svCache.lokiPerVbyte = estmFee.SatPerVbyte
	w.svCache.totalCost = totalcost
	w.svCache.fee = txFee
	w.svCache.balanceAfter = newBalance
	w.svCache.finalTx = nil
	w.mu.Unlock()
	feeField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(txFee, 6)))
	totalCostField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(totalcost, 6)))
	newBalanceField.SetText(fmt.Sprintf("[gray::]%s", shared.FormatAmountView(newBalance, 6)))
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
	}()
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

	for {
		select {
		case evt, ok := <-w.nsub:
			if !ok {
				return
			}
			w.handleNotification(evt)

		case <-w.quit:
			return
		}
	}
}

func (w *Wallet) handleNotification(evt *load.NotificationEvent) {

	switch evt.State {
	case flnwallet.StatusReady, flnwallet.StatusTransaction, flnwallet.StatusBlock:
		w.updateRows()
		return

	case flnwallet.StatusScanning:
		msg := "Scanning transactions..."
		if evt.BlockHeight > 0 {
			msg = fmt.Sprintf("Scanning transactions... (%d)", evt.BlockHeight)
		}
		w.showPlaceholder(msg)
		return

	case flnwallet.StatusSyncing:
		w.showPlaceholder("Syncing transactions...")
		return

	case flnwallet.StatusUnlocked:
		w.showPlaceholder("Unlocking wallet...")
		return

	case flnwallet.StatusNone:
		w.showPlaceholder("Connecting to wallet...")
		return

	case flnwallet.StatusNoWallet:
		w.showPlaceholder("Wallet not found.")
		return

	case flnwallet.StatusDown:
		w.showPlaceholder("Reconnecting to wallet...")
		return

	case flnwallet.StatusLocked:
		w.showPlaceholder("Wallet locked.")
		return
	}

	w.showPlaceholder("Loading transactions...")
}

func (w *Wallet) showPlaceholder(message string) {
	if message == "" {
		return
	}
	if !w.updatePlaceholderState(message) {
		return
	}
	w.load.Application.QueueUpdateDraw(func() {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
		if w.placeholder != message {
			return
		}
		w.table.ShowPlaceholder(message)
	})
}

func (w *Wallet) applyPlaceholder(message string) {
	if message == "" {
		return
	}
	if !w.updatePlaceholderState(message) {
		return
	}
	w.stateMu.Lock()
	defer w.stateMu.Unlock()
	if w.placeholder != message {
		return
	}
	w.table.ShowPlaceholder(message)
}

func (w *Wallet) updatePlaceholderState(message string) bool {
	w.stateMu.Lock()
	defer w.stateMu.Unlock()
	if w.placeholder == message {
		return false
	}
	w.placeholder = message
	return true
}

func (w *Wallet) clearPlaceholder() {
	w.stateMu.Lock()
	w.placeholder = ""
	w.stateMu.Unlock()
}

func (w *Wallet) updateRows() {
	rows := w.fetchTransactionsRows()
	w.load.Application.QueueUpdateDraw(func() {
		if rows == nil {
			return
		}
		if len(rows) == 0 {
			message := "No transactions yet."
			w.updatePlaceholderState(message)
			w.stateMu.Lock()
			defer w.stateMu.Unlock()
			if w.placeholder != message {
				return
			}
			w.table.ShowPlaceholder(message)
			return
		}
		w.clearPlaceholder()
		w.table.Update(rows)
	})
}

func (w *Wallet) Destroy() {
	w.quitOnce.Do(func() {
		if w.cancelN != nil {
			w.cancelN()
		}
		if w.logQuit != nil {
			close(w.logQuit)
			w.logQuit = nil
		}
		if w.quit != nil {
			close(w.quit)
		}
	})
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
