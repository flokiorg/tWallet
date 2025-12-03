// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package load

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/flokiorg/flnd/lnrpc"
	"github.com/flokiorg/go-flokicoin/chainutil"
	wlt "github.com/flokiorg/walletd/wallet"
	"github.com/gdamore/tcell/v2"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rivo/tview"

	"github.com/flokiorg/flnd/flnwallet"
	"github.com/flokiorg/twallet/config"
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
	logger := NamedLogger("load")

	l := &Load{
		Application: tapp,
		Nav:         newNavigator(tapp, pages),
		Wallet:      flnsvc,
		Logger:      logger,
		AppConfig:   cfg,
		Cache:       &Cache{},
	}

	l.Notif = newNotification(flnsvc, l.Cache, NamedLogger("notification"))

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
	wallet      *flnwallet.Service
	cache       *Cache
}

type NotificationEvent struct {
	AccountNotif   *wlt.AccountNotification
	TxNotif        *wlt.TransactionNotifications
	SpentNessNotif *wlt.SpentnessNotifications
	State          flnwallet.Status
	BlockHeight    uint32
	Err            error
}

type RecoveryStatus struct {
	Info      *lnrpc.GetRecoveryInfoResponse
	UTXOCount int
}

func (n *notification) Subscribe() (<-chan *NotificationEvent, func()) {
	ch := make(chan *NotificationEvent, 1)

	n.mu.Lock()
	n.subs = append(n.subs, ch)
	n.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			n.mu.Lock()
			for i := range n.subs {
				if n.subs[i] == ch {
					n.subs = append(n.subs[:i], n.subs[i+1:]...)
					break
				}
			}
			n.mu.Unlock()
			close(ch)
		})
	}

	return ch, unsubscribe
}

func newNotification(flnsvc *flnwallet.Service, cache *Cache, logger zerolog.Logger) *notification {
	n := &notification{
		toast:       make(chan string, 5),
		subs:        make([]chan *NotificationEvent, 0),
		stop:        make(chan struct{}),
		logger:      logger,
		cache:       cache,
		healthState: make(chan HealthState),
	}

	n.lnHealth = flnsvc.Subscribe()
	n.wallet = flnsvc

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

func (n *notification) BroadcastBalanceRefresh() {
	n.BroadcastWalletUpdate(nil)
}

func (l *Load) BroadcastBalanceRefresh() {
	if l == nil || l.Notif == nil {
		return
	}
	l.Notif.BroadcastBalanceRefresh()
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

	event := &NotificationEvent{
		State:       ev.State,
		BlockHeight: ev.BlockHeight,
		Err:         ev.Err,
	}

	switch ev.State {
	case flnwallet.StatusDown:
		n.reportHealth(HealthState{Level: HealthRed, Info: "disconnected", Err: ev.Err})
		n.BroadcastWalletUpdate(event)

	case flnwallet.StatusLocked:
		n.reportHealth(HealthState{Level: HealthOrange, Info: "locked"})
		n.BroadcastWalletUpdate(event)

	case flnwallet.StatusNone:
		info := "connecting..."
		n.reportHealth(HealthState{Level: HealthOrange, Info: info})
		n.BroadcastWalletUpdate(event)

	case flnwallet.StatusNoWallet:
		n.reportHealth(HealthState{Level: HealthOrange, Info: "no wallet"})
		n.BroadcastWalletUpdate(event)

	case flnwallet.StatusSyncing:
		var info string
		if ev.BlockHeight == 0 {
			info = "init..."
		} else {
			info = fmt.Sprintf("syncing... (%d)", ev.BlockHeight)
		}
		n.reportHealth(HealthState{Level: HealthOrange, Info: info})
		n.BroadcastWalletUpdate(event)

	case flnwallet.StatusUnlocked:
		n.reportHealth(HealthState{Level: HealthGreen, Info: "unlocked"})
		n.BroadcastWalletUpdate(event)

	case flnwallet.StatusReady:
		n.logger.Debug().
			Uint32("block_height", ev.BlockHeight).
			Msg("wallet ready event")
		if n.ensureWalletResponsive() {
			info := fmt.Sprintf("ready (%d)", ev.BlockHeight)
			n.cache.updateTip(int32(ev.BlockHeight))
			n.reportHealth(HealthState{Level: HealthGreen, Info: info})
			n.BroadcastWalletUpdate(event)
		} else {
			n.logger.Warn().Msg("wallet ready reported but RPC still unavailable")
			n.reportHealth(HealthState{Level: HealthOrange, Info: "waiting for wallet"})
			n.ShowToast("[red:-:-]Error:[-:-:-] wallet not ready")
		}

	case flnwallet.StatusTransaction:
		if ev.Transaction != nil {
			n.logger.Debug().
				Uint32("block_height", uint32(ev.Transaction.BlockHeight)).
				Int64("amount", ev.Transaction.Amount).
				Str("tx_hash", ev.Transaction.TxHash).
				Msg("transaction update received")
		} else {
			n.logger.Debug().Msg("transaction update received without payload")
		}
		n.cache.updateTip(ev.Transaction.BlockHeight)
		n.BroadcastWalletUpdate(event)

	case flnwallet.StatusBlock:
		n.logger.Debug().
			Uint32("block_height", ev.BlockHeight).
			Msg("new block notification")
		info := fmt.Sprintf("ready (%d)", ev.BlockHeight)
		n.cache.updateTip(int32(ev.BlockHeight))
		n.reportHealth(HealthState{Level: HealthGreen, Info: info})
		n.BroadcastWalletUpdate(event)

		// case flnwallet.StatusScanning:
		// 	var percent float64
		// 	if ev.SyncedHeight > 0 {
		// 		percent = float64(ev.BlockHeight) / float64(ev.SyncedHeight) * 100
		// 	}
		// 	n.logger.Debug().
		// 		Uint32("block_height", ev.BlockHeight).
		// 		Uint32("synced_height", ev.SyncedHeight).
		// 		Float64("progress", percent).
		// 		Msg("wallet scanning update")
		// 	info := fmt.Sprintf("Scanning... %d (%.0f%%)", ev.SyncedHeight, percent)
		// 	n.reportHealth(HealthState{Level: HealthGreen, Info: info})
		// 	n.BroadcastWalletUpdate(event)
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
	n.subs = nil
}

func (n *notification) Health() <-chan HealthState {
	return n.healthState
}

func (n *notification) Toast() <-chan string {
	return n.toast
}

func (n *notification) ensureWalletResponsive() bool {
	const (
		maxAttempts = 5
		delay       = 300 * time.Millisecond
	)

	if n.wallet == nil {
		return true
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		_, err := n.wallet.Balance()
		if err == nil {
			n.logger.Debug().Msg("wallet responsive confirmed")
			return true
		}

		st, ok := status.FromError(err)
		if ok {
			switch st.Code() {
			case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled:
				n.logger.Debug().Err(err).Msg("wallet RPC not ready yet")
				time.Sleep(delay)
				continue
			}
		}

		n.logger.Error().Err(err).Msg("wallet balance failed")
		n.ShowToast(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()))
		return false
	}

	n.ShowToast("[red:-:-]Error:[-:-:-] wallet not ready")
	return false
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
	lockedBalance      chainutil.Amount
	confirmedBalance   chainutil.Amount
	unconfirmedBalance chainutil.Amount
	tipHeight          int32
	mu                 sync.Mutex
}

func (c *Cache) SetBalance(confirmedBalance, unconfirmedBalance, lockedBalance chainutil.Amount) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.confirmedBalance = chainutil.Amount(confirmedBalance)
	c.lockedBalance = chainutil.Amount(lockedBalance)
	c.unconfirmedBalance = chainutil.Amount(unconfirmedBalance)
}

func (c *Cache) GetBalance() (chainutil.Amount, chainutil.Amount, chainutil.Amount) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.confirmedBalance, c.unconfirmedBalance, c.lockedBalance
}

func (c *Cache) updateTip(height int32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if height > c.tipHeight {
		c.tipHeight = height
	}
}

func (c *Cache) GetTipHeight() int32 {
	c.mu.Lock()
	tip := c.tipHeight
	c.mu.Unlock()

	return tip
}

func (l *Load) GetRecoveryStatus() (*RecoveryStatus, error) {
	info, err := l.Wallet.GetRecoveryInfo()
	if err != nil {
		return nil, err
	}
	utxos, err := l.Wallet.ListUnspent(0, math.MaxInt32)
	if err != nil {
		return nil, err
	}
	return &RecoveryStatus{Info: info, UTXOCount: len(utxos)}, nil
}

func (l *Load) MonitorRecovery(ctx context.Context, interval time.Duration, cb func(*RecoveryStatus) bool) (*RecoveryStatus, error) {
	if interval <= 0 {
		interval = time.Second
	}
	for {
		status, err := l.GetRecoveryStatus()
		if err != nil {
			if errors.Is(err, flnwallet.ErrDaemonNotRunning) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(interval):
					continue
				}
			}
			return nil, err
		}
		cont := true
		if cb != nil {
			cont = cb(status)
		}
		if !cont || status.Info == nil || status.Info.GetRecoveryFinished() || status.Info.GetProgress() >= 1 {
			return status, nil
		}
		select {
		case <-ctx.Done():
			return status, ctx.Err()
		case <-time.After(interval):
		}
	}
}
