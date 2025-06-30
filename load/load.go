// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package load

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/flokiorg/flnd/lnrpc"
	"github.com/flokiorg/go-flokicoin/chainutil"
	wlt "github.com/flokiorg/walletd/wallet"
	"github.com/gdamore/tcell/v2"
	"github.com/rs/zerolog"

	"github.com/rivo/tview"

	"github.com/flokiorg/flnd/flnwallet"
	"github.com/flokiorg/twallet/load/config"
	. "github.com/flokiorg/twallet/shared"
)

type HealthLevel int

const (
	HealthRed    HealthLevel = iota // 0 - Critical/Down
	HealthOrange                    // 1 - Degraded
	HealthGreen                     // 2 - Healthy
)

type HealthState struct {
	Level HealthLevel
	Info  string
	Err   error
}

type Router interface {
	Go(Page)
}

type Load struct {
	*tview.Application
	*Cache
	Router
	Nav       *Navigator
	Notif     *notification
	Wallet    *flnwallet.Service
	Logger    zerolog.Logger
	AppConfig *config.AppConfig
}

func NewLoad(cfg *config.AppConfig, flnsvc *flnwallet.Service, tapp *tview.Application, pages *tview.Pages) *Load {

	logger := CreateFileLogger(filepath.Join(cfg.Walletdir, "twallet.log"))

	l := &Load{
		Application: tapp,
		Nav:         newNavigator(tapp, pages),
		Wallet:      flnsvc,
		Notif:       newNotification(flnsvc, logger),
		Logger:      logger,
		AppConfig:   cfg,
		Cache:       &Cache{},
	}

	l.Application.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() != tcell.KeyESC {
			return event
		}
		l.Notif.CancelToast()
		l.Application.SetFocus(pages)
		return event
	})

	return l
}

func (l *Load) RegisterRouter(r Router) {
	l.Router = r
}

type notification struct {
	toast chan string

	mu     sync.Mutex
	subs   []chan *NotificationEvent
	stop   chan struct{}
	logger zerolog.Logger

	healthState chan HealthState
	lnHealth    <-chan *flnwallet.Update

	lastHeight uint32
}

type NotificationEvent struct {
	AccountNotif   *wlt.AccountNotification
	TxNotif        *wlt.TransactionNotifications
	SpentNessNotif *wlt.SpentnessNotifications
}

func (n *notification) Subscribe() <-chan *NotificationEvent {
	n.mu.Lock()
	defer n.mu.Unlock()

	ch := make(chan *NotificationEvent)
	n.subs = append(n.subs, ch)
	return ch
}

func newNotification(flnsvc *flnwallet.Service, logger zerolog.Logger) *notification {
	n := &notification{
		toast:  make(chan string, 5),
		subs:   make([]chan *NotificationEvent, 0),
		stop:   make(chan struct{}),
		logger: logger,

		healthState: make(chan HealthState),
	}

	n.lnHealth = flnsvc.Subscribe()

	go n.listen()

	return n
}

func (n *notification) BroadcastWalletUpdate(event *NotificationEvent) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for _, ch := range n.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func (n *notification) listen() {

	for {
		select {
		case ev, ok := <-n.lnHealth:
			if !ok {
				return
			}
			n.ProcessEvent(ev)

		case <-n.stop:
			return
		}
	}
}

func (n *notification) ProcessEvent(ev *flnwallet.Update) {

	switch ev.State {
	case flnwallet.StatusDown:
		n.reportHealth(HealthState{Level: HealthRed, Info: "disconnected", Err: ev.Err})

	case flnwallet.StatusLocked:
		n.reportHealth(HealthState{Level: HealthOrange, Info: "locked"})

	case flnwallet.StatusNone:
		info := "connecting..."
		n.reportHealth(HealthState{Level: HealthOrange, Info: info})

	case flnwallet.StatusNoWallet:
		n.reportHealth(HealthState{Level: HealthOrange, Info: "no wallet"})

	case flnwallet.StatusSyncing:
		var info string
		if ev.BlockHeight == 0 {
			info = "syncing..."
		} else {
			info = fmt.Sprintf("syncing... (%d)", ev.BlockHeight)
		}
		n.reportHealth(HealthState{Level: HealthOrange, Info: info})

	case flnwallet.StatusUnlocked:
		n.reportHealth(HealthState{Level: HealthGreen, Info: "unlocked"})

	case flnwallet.StatusReady:
		info := fmt.Sprintf("ready (%d)", ev.BlockHeight)
		n.logger.Debug().Msgf("wallet ready block: %v", ev.BlockHeight)
		n.reportHealth(HealthState{Level: HealthGreen, Info: info})
		n.BroadcastWalletUpdate(&NotificationEvent{})

	case flnwallet.StatusTransaction:
		n.logger.Debug().Msgf("new tx height:%v amount:%v", ev.Transaction.BlockHeight, ev.Transaction.Amount)
		n.BroadcastWalletUpdate(&NotificationEvent{})

	case flnwallet.StatusBlock:
		n.logger.Debug().Msgf("new block: %v", ev.BlockHeight)
		n.lastHeight = ev.BlockHeight
		info := fmt.Sprintf("ready (%d)", ev.BlockHeight)
		n.reportHealth(HealthState{Level: HealthGreen, Info: info})

	case flnwallet.StatusScanning:
		n.logger.Debug().Msgf("new sync block: %v", ev.BlockHeight)
		percent := float64(ev.BlockHeight) / float64(ev.SyncedHeight) * 100
		info := fmt.Sprintf("Scanning... %d (%.0f%%)", ev.SyncedHeight, percent)
		n.reportHealth(HealthState{Level: HealthGreen, Info: info})
	}
}

func (n *notification) reportHealth(h HealthState) {
	select {
	case n.healthState <- h:
	case <-time.After(time.Second * 5):
	}
}

func (n *notification) Shutdown() {
	n.mu.Lock()
	defer n.mu.Unlock()

	close(n.stop)
	for _, ch := range n.subs {
		close(ch)
	}
}

func (n *notification) Health() <-chan HealthState {
	return n.healthState
}

func (n *notification) Toast() <-chan string {
	return n.toast
}

func (n *notification) ShowToast(text string) {
	select {
	case n.toast <- text:
	default:
	}
}

func (n *notification) CancelToast() {
	select {
	case n.toast <- "":
	default:
	}
}

func (n *notification) ShowToastWithTimeout(text string, d time.Duration) {
	n.toast <- text
	go func() {
		time.Sleep(d)
		n.toast <- ""
	}()
}

type Cache struct {
	balance chainutil.Amount
	mu      sync.Mutex
}

func (c *Cache) SetBalance(resp *lnrpc.WalletBalanceResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.balance = chainutil.Amount(resp.TotalBalance)
}

func (c *Cache) GetBalance() chainutil.Amount {
	c.mu.Lock()
	balance := c.balance
	c.mu.Unlock()
	return balance
}
