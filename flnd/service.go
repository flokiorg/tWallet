package flnd

import (
	"context"
	"sync"
	"time"

	"github.com/flokiorg/flnd"
	"github.com/flokiorg/flnd/lncfg"
	"github.com/flokiorg/flnd/lnrpc"
	"github.com/flokiorg/flnd/lnrpc/walletrpc"
	"github.com/flokiorg/flnd/signal"
	"github.com/flokiorg/go-flokicoin/chaincfg"
	"github.com/flokiorg/go-flokicoin/chainutil"
	"github.com/flokiorg/go-flokicoin/chainutil/psbt"
)

type Status string

const (
	StatusInit        Status = "init"
	StatusNone        Status = "none"
	StatusLocked      Status = "locked"
	StatusUnlocked    Status = "unlocked"
	StatusSyncing     Status = "syncing"
	StatusReady       Status = "ready"
	StatusNoWallet    Status = "noWallet"
	StatusDown        Status = "down"
	StatusTransaction Status = "tx"
	StatusBlock       Status = "block"
	StatusScanning    Status = "scanning"
	StatusQuit        Status = "quit"
)

type Update struct {
	State                     Status
	Err                       error
	Transaction               *lnrpc.Transaction
	BlockHeight, SyncedHeight uint32
	BlockHash                 string
}

type OutputLock struct {
	ID       []byte
	Outpoint *lnrpc.OutPoint
}

type FundedPsbt struct {
	Packet *psbt.Packet
	Locks  []*OutputLock
}

type ServiceConfig struct {
	Walletdir               string        `short:"w" long:"walletdir"  description:"Directory for Flokicoin Lightning Network"`
	RegressionTest          bool          `long:"regtest" description:"Use the regression test network"`
	Testnet                 bool          `long:"testnet" description:"Use the test network"`
	ConnectionTimeout       time.Duration `short:"t" long:"connectiontimeout" default:"50s" description:"The timeout value for network connections. Valid time units are {ms, s, m, h}."`
	DebugLevel              string        `short:"d" long:"debuglevel" default:"info" description:"Logging level for all subsystems {trace, debug, info, warn, error, critical}"`
	ConnectPeers            []string      `long:"connect" description:"Connect only to the specified peers at startup"`
	Feeurl                  string        `long:"feeurl" description:"Custom fee estimation API endpoint (Required on mainnet)"`
	TransactionDisplayLimit int           `long:"transactiondisplaylimit" description:"Maximum number of transactions to fetch per request"`
	ResetWalletTransactions bool          `long:"resetwallettransactions" description:"Reset wallet transactions on startup to trigger a full rescan"`

	TLSExtraIPs     []string `long:"tlsextraip" description:"Adds an extra ip to the generated certificate"`
	TLSExtraDomains []string `long:"tlsextradomain" description:"Adds an extra domain to the generated certificate"`
	TLSAutoRefresh  bool     `long:"tlsautorefresh" description:"Re-generate TLS certificate and key if the IPs or domains are changed"`

	RawRPCListeners  []string `long:"rpclisten" description:"Add an interface/port/socket to listen for RPC connections"`
	RawRESTListeners []string `long:"restlisten" description:"Add an interface/port/socket to listen for REST connections"`
	RawListeners     []string `long:"listen" description:"Add an interface/port to listen for peer connections"`

	RestCORS []string `long:"restcors" description:"Add an ip:port/hostname to allow cross origin access from. To allow all origins, set as \"*\"."`

	Network *chaincfg.Params
}

type Service struct {
	subMu sync.Mutex
	subs  []chan *Update

	ctx    context.Context
	cancel context.CancelFunc

	flndConfig           *flnd.Config
	configMu             sync.Mutex
	client               *Client
	daemon               *daemon
	cmux                 sync.Mutex
	wg                   sync.WaitGroup
	running              bool
	lastEvent            *Update
	maxTransactionsLimit uint32
	stopOnce             sync.Once
}

func New(pctx context.Context, cfg *ServiceConfig) *Service {

	ctx, cancel := context.WithCancel(pctx)

	conf := flnd.DefaultConfig()
	conf.LndDir = cfg.Walletdir
	conf.Flokicoin.Node = "neutrino"
	conf.NeutrinoMode.ConnectPeers = cfg.ConnectPeers
	conf.DebugLevel = cfg.DebugLevel
	conf.Fee.URL = cfg.Feeurl
	conf.ProtocolOptions = &lncfg.ProtocolOptions{}
	conf.Pprof = &lncfg.Pprof{}
	conf.LogConfig.Console.Disable = true
	conf.ConnectionTimeout = cfg.ConnectionTimeout
	conf.TLSExtraDomains = append(conf.TLSExtraDomains, cfg.TLSExtraDomains...)
	conf.TLSExtraIPs = append(conf.TLSExtraIPs, cfg.TLSExtraIPs...)
	conf.RawRPCListeners = append(conf.RawRPCListeners, cfg.RawRPCListeners...)
	conf.RawRESTListeners = append(conf.RawRESTListeners, cfg.RawRESTListeners...)
	conf.RawListeners = append(conf.RawListeners, cfg.RawListeners...)
	conf.RestCORS = append(conf.RestCORS, cfg.RestCORS...)
	conf.TLSAutoRefresh = cfg.TLSAutoRefresh
	conf.ResetWalletTransactions = cfg.ResetWalletTransactions
	switch cfg.Network {
	case &chaincfg.MainNetParams:
		conf.Flokicoin.MainNet = true
	case &chaincfg.TestNet3Params:
		conf.Flokicoin.TestNet3 = true
	case &chaincfg.TestNet4Params:
		conf.Flokicoin.TestNet4 = true
	case &chaincfg.SimNetParams:
		conf.Flokicoin.SigNet = true
	case &chaincfg.RegressionNetParams:
		conf.Flokicoin.RegTest = true
	case &chaincfg.SigNetParams:
		conf.Flokicoin.SigNet = true
	}

	s := &Service{
		lastEvent:            &Update{State: StatusInit},
		flndConfig:           &conf,
		ctx:                  ctx,
		cancel:               cancel,
		maxTransactionsLimit: uint32(cfg.TransactionDisplayLimit),
	}

	go s.run()

	return s
}

func (s *Service) run() {
	s.wg.Add(1)
	defer s.wg.Done()

	retryDelay := time.Second
	const maxRetryDelay = 30 * time.Second

	for {
		select {
		case <-s.ctx.Done():
			s.stopDaemon()
			return

		default:

			s.notifySubscribers(&Update{State: StatusNone})
			interceptor, err := signal.Intercept()
			if err != nil {
				s.notifySubscribers(&Update{State: StatusDown, Err: err})
				if !s.waitForRetry(retryDelay) {
					return
				}
				if retryDelay < maxRetryDelay {
					retryDelay = retryDelay * 2
					if retryDelay > maxRetryDelay {
						retryDelay = maxRetryDelay
					}
				}
				continue
			}

			d, err := newDaemon(s.ctx, s.cloneConfig(), interceptor)
			if err != nil {
				s.notifySubscribers(&Update{State: StatusDown, Err: err})
				if !s.waitForRetry(retryDelay) {
					return
				}
				if retryDelay < maxRetryDelay {
					retryDelay = retryDelay * 2
					if retryDelay > maxRetryDelay {
						retryDelay = maxRetryDelay
					}
				}
				continue
			}
			c, err := d.start()
			if err != nil {
				s.notifySubscribers(&Update{State: StatusDown, Err: err})
				if !s.waitForRetry(retryDelay) {
					return
				}
				if retryDelay < maxRetryDelay {
					retryDelay = retryDelay * 2
					if retryDelay > maxRetryDelay {
						retryDelay = maxRetryDelay
					}
				}
				continue
			}
			retryDelay = time.Second
			s.running = true
			ctx, cancel := context.WithCancel(s.ctx)
			go func() {
				for {

					select {
					case <-ctx.Done():
						d.stop()
						return

					case health := <-c.Health():
						s.notifySubscribers(health)
						switch health.State {
						case StatusDown:
							d.stop()
						default:
						}
					}
				}
			}()
			s.registerConnection(d, c)
			d.waitForShutdown()
			cancel()
			s.running = false
		}
	}
}

func (s *Service) waitForRetry(delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-s.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *Service) cloneConfig() *flnd.Config {
	s.configMu.Lock()
	defer s.configMu.Unlock()

	cfg := *s.flndConfig
	cfg.TLSExtraDomains = append([]string(nil), s.flndConfig.TLSExtraDomains...)
	cfg.TLSExtraIPs = append([]string(nil), s.flndConfig.TLSExtraIPs...)
	cfg.RawRPCListeners = append([]string(nil), s.flndConfig.RawRPCListeners...)
	cfg.RawRESTListeners = append([]string(nil), s.flndConfig.RawRESTListeners...)
	cfg.RawListeners = append([]string(nil), s.flndConfig.RawListeners...)
	cfg.RestCORS = append([]string(nil), s.flndConfig.RestCORS...)
	cfg.NeutrinoMode.ConnectPeers = append([]string(nil), s.flndConfig.NeutrinoMode.ConnectPeers...)
	return &cfg
}

func (s *Service) Stop() {
	s.stopOnce.Do(func() {
		s.stopDaemon()
		s.cancel()
		s.unsubscribeAll()
		s.wg.Wait()
		s.running = false
	})
}

func (s *Service) stopDaemon() {
	s.cmux.Lock()
	defer s.cmux.Unlock()

	if s.daemon != nil {
		s.daemon.stop()
		s.daemon.waitForShutdown()
		s.daemon = nil
		s.client = nil
	}
}

func (s *Service) Restart(pctx context.Context) {
	if s.daemon != nil {
		s.daemon.stop()
	}
}

func (s *Service) registerConnection(d *daemon, c *Client) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	s.client = c
	s.daemon = d
	c.SetMaxTransactionsLimit(s.maxTransactionsLimit)
	s.configMu.Lock()
	s.flndConfig.ResetWalletTransactions = false
	s.configMu.Unlock()
}

func (s *Service) TriggerRescan() error {
	s.configMu.Lock()
	s.flndConfig.ResetWalletTransactions = true
	s.configMu.Unlock()

	s.Restart(context.Background())
	return nil
}

func (s *Service) GetRecoveryInfo() (*lnrpc.GetRecoveryInfoResponse, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return nil, ErrDaemonNotRunning
	}
	return s.client.GetRecoveryInfo()
}

func (s *Service) ListUnspent(minConfs, maxConfs int32) ([]*lnrpc.Utxo, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return nil, ErrDaemonNotRunning
	}
	return s.client.ListUnspent(minConfs, maxConfs)
}

func (s *Service) VerifyMessage(address, message, signature string) (*walletrpc.VerifyMessageWithAddrResponse, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return nil, ErrDaemonNotRunning
	}
	return s.client.VerifyMessageWithAddress(address, message, signature)
}

func (s *Service) Subscribe() <-chan *Update {
	ch := make(chan *Update, 5)
	s.subMu.Lock()
	s.subs = append(s.subs, ch)
	ch <- s.lastEvent
	s.subMu.Unlock()
	return ch
}

func (s *Service) Unsubscribe(ch <-chan *Update) {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	for i := 0; i < len(s.subs); i++ {
		if s.subs[i] == ch {
			s.subs = append(s.subs[:i], s.subs[i+1:]...)
			break
		}
	}
}

func (s *Service) notifySubscribers(u *Update) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	s.lastEvent = u

	for _, ch := range s.subs {
		select {
		case ch <- u:
		default:
		}
	}
}

func (s *Service) unsubscribeAll() {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	if len(s.subs) == 0 {
		return
	}

	finalUpdate := &Update{
		State: StatusDown,
	}

	for _, ch := range s.subs {
		select {
		case ch <- finalUpdate:
		case <-time.After(5 * time.Second):
		}
		close(ch)
	}

	s.subs = s.subs[:0]
}

func (s *Service) CreateWallet(passphrase string) (string, []string, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return "", nil, ErrDaemonNotRunning
	}
	return s.client.Create(passphrase)
}

func (s *Service) Balance() (*lnrpc.WalletBalanceResponse, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return nil, ErrDaemonNotRunning
	}
	return s.client.Balance()
}

func (s *Service) IsLocked() (bool, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return false, ErrDaemonNotRunning
	}
	return s.client.IsLocked()
}

func (s *Service) Unlock(passphrase string) error {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return ErrDaemonNotRunning
	}
	return s.client.Unlock(passphrase)
}

func (s *Service) WalletExists() (bool, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return false, ErrDaemonNotRunning
	}
	return s.client.WalletExists()
}

func (s *Service) FetchTransactions() ([]*lnrpc.Transaction, error) {
	return s.FetchTransactionsWithOptions(FetchTransactionsOptions{})
}

func (s *Service) FetchTransactionsWithOptions(opts FetchTransactionsOptions) ([]*lnrpc.Transaction, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return nil, ErrDaemonNotRunning
	}
	return s.client.FetchTransactionsWithOptions(opts)
}

func (s *Service) GetNextAddress(t lnrpc.AddressType) (chainutil.Address, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return nil, ErrDaemonNotRunning
	}
	return s.client.GetNextAddress(t)
}

func (s *Service) SignMessage(address string, message string) (string, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return "", ErrDaemonNotRunning
	}
	return s.client.SignMessageWithAddress(address, message)
}

func (s *Service) ListAddresses() ([]*walletrpc.AccountWithAddresses, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return nil, ErrDaemonNotRunning
	}
	return s.client.ListAddresses()
}

func (s *Service) RestoreByMnemonic(mnemonic []string, passphrase string) (string, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return "", ErrDaemonNotRunning
	}
	return s.client.RestoreByMnemonic(mnemonic, passphrase)
}

func (s *Service) RestoreByEncipheredSeed(strEncipheredSeed, passphrase string) ([]string, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return nil, ErrDaemonNotRunning
	}
	return s.client.RestoreByEncipheredSeed(strEncipheredSeed, passphrase)
}

func (s *Service) ChangePassphrase(old, new string) error {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return ErrDaemonNotRunning
	}
	return s.client.ChangePassphrase(old, new)
}

func (s *Service) Transfer(address chainutil.Address, amount chainutil.Amount, lokiPerVbyte uint64) (string, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return "", ErrDaemonNotRunning
	}
	return s.client.SimpleTransfer(address, amount, lokiPerVbyte)
}

func (s *Service) Fee(address chainutil.Address, amount chainutil.Amount) (*lnrpc.EstimateFeeResponse, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return nil, ErrDaemonNotRunning
	}
	return s.client.SimpleTransferFee(address, amount)
}

func (s *Service) FundPsbt(addrToAmount map[string]int64, lokiPerVbyte uint64, lockExpirationSeconds uint64) (*FundedPsbt, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return nil, ErrDaemonNotRunning
	}
	return s.client.FundPsbt(addrToAmount, lokiPerVbyte, lockExpirationSeconds)
}

func (s *Service) FinalizePsbt(packet *psbt.Packet) (*chainutil.Tx, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return nil, ErrDaemonNotRunning
	}
	return s.client.FinalizePsbt(packet)
}

func (s *Service) PublishTransaction(tx *chainutil.Tx) error {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return ErrDaemonNotRunning
	}
	return s.client.PublishTransaction(tx)
}

func (s *Service) ReleaseOutputs(locks []*OutputLock) error {
	if len(locks) == 0 {
		return nil
	}
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return ErrDaemonNotRunning
	}
	return s.client.ReleaseOutputs(locks)
}

func (s *Service) GetLastEvent() *Update {
	return s.lastEvent
}

func (s *Service) GetLightningConfig() (*LightningConfig, error) {
	s.cmux.Lock()
	defer s.cmux.Unlock()
	if s.client == nil {
		return nil, ErrDaemonNotRunning
	}
	return s.client.GetLightningConfig()
}
