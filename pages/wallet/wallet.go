// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package wallet

import (
	"sync"
	"unicode"

	"github.com/rivo/tview"

	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/shared"
	"github.com/gdamore/tcell/v2"
)

type walletView int

const (
	transactionsView walletView = iota
	logsView
)

const (
	transactionsPageName = "transactions"
	logsPageName         = "logs"
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
	txRetryMu   sync.Mutex
	placeholder string

	svCache          *sendViewModel
	quit             chan struct{}
	busy             bool
	rescanInProgress bool
	nsub             <-chan *load.NotificationEvent
	cancelN          func()
	txRetryHandle    *txRetryHandle
	quitOnce         sync.Once

	logLines   []string
	logQuit    chan struct{}
	logPath    string
	logReady   bool
	logMaxLine int
	logStatus  string
	onceReady  sync.Once
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

	table := components.NewTable("Transactions", columns, netColor, l.AppConfig.TransactionDisplayLimit)
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
	case tcell.KeyCtrlS:
		w.showMessageTools()
		return nil
	case tcell.KeyCtrlA:
		w.showUsedAddresses()
		return nil
	case tcell.KeyCtrlX:
		w.promptRescan()
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

func (w *Wallet) Destroy() {
	w.quitOnce.Do(func() {
		w.cancelTransactionsUpdateRetry()
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
