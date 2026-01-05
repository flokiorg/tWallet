package wallet

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/flokiorg/flnd/lnrpc"
	"github.com/flokiorg/go-flokicoin/chainutil"

	"github.com/flokiorg/twallet/flnd"
	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/shared"
)

const transactionsUpdateRetryInterval = 5 * time.Second

type txRetryHandle struct {
	cancel context.CancelFunc
}

func (w *Wallet) fetchTransactionsRows() [][]string {
	tipHeight := w.load.Cache.GetTipHeight()
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
		numConfirmations := int64(tipHeight - tx.BlockHeight + 1)
		if tx.BlockHeight < 1 {
			numConfirmations = 0
		}

		if numConfirmations < 1 {
			row = append(row, strconv.FormatInt(0, 10))
		} else {
			row = append(row, strconv.FormatInt(numConfirmations, 10))
		}
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

	if evt == nil {
		return
	}

	w.cancelTransactionsUpdateRetry()

	switch evt.State {
	case flnd.StatusReady, flnd.StatusTransaction, flnd.StatusBlock:
		w.onceReady.Do(func() {
			w.showPlaceholder("Loading transactions...")
		})
		if !w.updateRows() {
			w.scheduleTransactionsUpdateRetry()
		}
		return

	case flnd.StatusScanning:
		msg := "Scanning transactions..."
		if evt.BlockHeight > 0 {
			msg = fmt.Sprintf("Scanning transactions... (%d)", evt.BlockHeight)
		}
		w.showPlaceholder(msg)
		return

	case flnd.StatusSyncing:
		msg := "Syncing transactions..."

		if rs, err := w.load.GetRecoveryStatus(); err == nil && rs != nil && rs.Info != nil {
			progress := rs.Info.GetProgress()
			switch {
			case rs.Info.GetRecoveryFinished():
				msg = "Syncing transactions... finalizing recovery"

			case progress > 1:
				recoveryPercent := (2 - progress) * 100
				if recoveryPercent < 0 {
					recoveryPercent = 0
				}
				msg = fmt.Sprintf("Syncing transactions... recovering... %.2f%% complete", recoveryPercent)
			case progress > 0:
				msg = fmt.Sprintf("Syncing transactions... %.2f%% complete", progress*100)
			}
		}

		w.showPlaceholder(msg)
		return

	case flnd.StatusUnlocked:
		w.showPlaceholder("Unlocking wallet...")
		return

	case flnd.StatusNone:
		w.showPlaceholder("Connecting to wallet...")
		return

	case flnd.StatusNoWallet:
		w.showPlaceholder("Wallet not found.")
		return

	case flnd.StatusDown:
		w.showPlaceholder("Reconnecting to wallet...")
		return

	case flnd.StatusLocked:
		if w.isRescanActive() {
			w.showPlaceholder("Wallet rescan: waiting for unlock...")
			return
		}
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

func (w *Wallet) updateRows() bool {
	rows := w.fetchTransactionsRows()
	if rows == nil {
		return false
	}
	w.load.Application.QueueUpdateDraw(func() {
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
	return true
}

func (w *Wallet) scheduleTransactionsUpdateRetry() {
	w.cancelTransactionsUpdateRetry()

	ctx, cancel := context.WithCancel(context.Background())
	handle := &txRetryHandle{cancel: cancel}

	w.txRetryMu.Lock()
	w.txRetryHandle = handle
	w.txRetryMu.Unlock()

	go w.runTransactionsUpdateRetry(ctx, handle)
}

func (w *Wallet) runTransactionsUpdateRetry(ctx context.Context, handle *txRetryHandle) {
	ticker := time.NewTicker(transactionsUpdateRetryInterval)
	defer func() {
		ticker.Stop()
		if handle != nil && handle.cancel != nil {
			handle.cancel()
		}
		w.txRetryMu.Lock()
		if w.txRetryHandle == handle {
			w.txRetryHandle = nil
		}
		w.txRetryMu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.quit:
			return
		case <-ticker.C:
			if w.updateRows() {
				return
			}
		}
	}
}

func (w *Wallet) cancelTransactionsUpdateRetry() {
	w.txRetryMu.Lock()
	handle := w.txRetryHandle
	w.txRetryHandle = nil
	w.txRetryMu.Unlock()

	if handle != nil && handle.cancel != nil {
		handle.cancel()
	}
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
