package flnd

import (
	"context"
	"sync"
	"time"

	"github.com/flokiorg/flnd"
	"github.com/flokiorg/flnd/lncfg"
	"github.com/flokiorg/flnd/lnrpc"
	"github.com/flokiorg/flnd/lnrpc/walletrpc"
	"github.com/flokiorg/flnd/lnwire"
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
	// Basic Configuration
	Walletdir               string        `short:"w" long:"walletdir" description:"Directory for Flokicoin Lightning Network"`
	RegressionTest          bool          `long:"regtest" description:"Use the regression test network"`
	Testnet                 bool          `long:"testnet" description:"Use the test network"`
	ConnectionTimeout       time.Duration `short:"t" long:"connectiontimeout" default:"50s" description:"The timeout value for network connections. Valid time units are {ms, s, m, h}."`
	DebugLevel              string        `short:"d" long:"debuglevel" default:"info" description:"Logging level for all subsystems {trace, debug, info, warn, error, critical}"`
	TransactionDisplayLimit int           `long:"transactiondisplaylimit" description:"Maximum number of transactions to fetch per request"`
	ResetWalletTransactions bool          `long:"resetwallettransactions" description:"Reset wallet transactions on startup to trigger a full rescan"`

	// Network & Peers
	ConnectPeers []string `long:"connect" description:"Connect only to the specified peers at startup"`
	AddPeers     []string `long:"addpeer" description:"Add peers to connect to at startup"`

	// Fee Configuration
	Feeurl string `long:"feeurl" description:"Custom fee estimation API endpoint (Required on mainnet)"`

	// TLS Configuration
	TLSExtraIPs     []string `long:"tlsextraip" description:"Adds an extra ip to the generated certificate"`
	TLSExtraDomains []string `long:"tlsextradomain" description:"Adds an extra domain to the generated certificate"`
	TLSAutoRefresh  bool     `long:"tlsautorefresh" description:"Re-generate TLS certificate and key if the IPs or domains are changed"`

	// RPC/REST Listeners
	RawRPCListeners  []string `long:"rpclisten" description:"Add an interface/port/socket to listen for RPC connections"`
	RawRESTListeners []string `long:"restlisten" description:"Add an interface/port/socket to listen for REST connections"`
	RawListeners     []string `long:"listen" description:"Add an interface/port to listen for peer connections"`
	RestCORS         []string `long:"restcors" description:"Add an ip:port/hostname to allow cross origin access from. To allow all origins, set as \"*\"."`

	// Channel Configuration
	MaxPendingChannels int   `long:"maxpendingchannels" description:"The maximum number of incoming pending channels permitted per peer"`
	MaxChanSize        int64 `long:"maxchansize" description:"The largest channel size (in satoshis) that we should accept"`
	MinChanSize        int64 `long:"minchansize" description:"The smallest channel size (in satoshis) that we should accept"`

	// Node Identity
	Alias string `long:"alias" description:"The node alias (max 32 UTF-8 characters)"`
	Color string `long:"color" description:"The color of the node in hex format (i.e. '#da9526'). Used to customize node appearance in graph visualizations"`

	// Watchtower
	WatchtowerActive bool   `long:"watchtower" description:"Enable integrated watchtower"`
	WatchtowerDir    string `long:"watchtower.towerdir" description:"Directory for watchtower state"`

	// Public Node Configuration
	ExternalIPs   []string `long:"externalip" description:"Add an ip:port to advertise to the network for incoming connections"`
	ExternalHosts []string `long:"externalhosts" description:"Add a hostname:port that should be periodically resolved to announce IPs for. If port is not specified, the default (9735) will be used"`
	DisableListen bool     `long:"nolisten" description:"Disable listening for incoming peer connections"`
	NAT           bool     `long:"nat" description:"Toggle NAT traversal support (using either UPnP or NAT-PMP) to automatically advertise your external IP address to the network"`

	// Routing & Forwarding
	MinHTLC       int64  `long:"minhtlc" description:"The smallest HTLC we will forward (in millisatoshis)"`
	BaseFee       int64  `long:"basefee" description:"The base fee in millisatoshi we will charge for forwarding payments on our channels"`
	FeeRate       int64  `long:"feerate" description:"The fee rate used when forwarding payments on our channels (in millionths)"`
	TimeLockDelta uint32 `long:"timelockdelta" description:"The CLTV delta we will subtract from a forwarded HTLC's timelock value"`
	AcceptKeySend bool   `long:"accept-keysend" description:"If true, spontaneous payments through keysend will be accepted"`
	AcceptAMP     bool   `long:"accept-amp" description:"If true, spontaneous payments through AMP will be accepted"`
	Wumbo         bool   `long:"wumbo-channels" description:"If true, the node will be configured to allow channels larger than 5 FLC"`

	// Network Graph & Gossip
	NumGraphSyncPeers       int           `long:"numgraphsyncpeers" description:"The number of peers that we should receive new graph updates from"`
	HistoricalSyncInterval  time.Duration `long:"historicalsyncinterval" description:"The polling interval between historical graph sync attempts"`
	IgnoreHistoricalFilters bool          `long:"ignore-historical-gossip-filters" description:"If true, will not reply with historical data that matches the range specified by a remote peer's gossip_timestamp_filter"`
	RejectHTLC              bool          `long:"rejecthtlc" description:"If true, lnd will not forward any HTLCs that are meant as onward payments"`
	StaggerInitialReconnect bool          `long:"stagger-initial-reconnect" description:"If true, will apply a randomized staggering between 0s and 30s when reconnecting to persistent peers on startup"`
	MaxOutgoingCltvExpiry   uint32        `long:"max-cltv-expiry" description:"The maximum number of blocks funds could be locked up for when forwarding payments"`

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

	// Network & Peers
	conf.NeutrinoMode.AddPeers = append(conf.NeutrinoMode.AddPeers, cfg.AddPeers...)

	// Channel Configuration
	if cfg.MaxPendingChannels > 0 {
		conf.MaxPendingChannels = cfg.MaxPendingChannels
	}
	if cfg.MaxChanSize > 0 {
		conf.MaxChanSize = cfg.MaxChanSize
	}
	if cfg.MinChanSize > 0 {
		conf.MinChanSize = cfg.MinChanSize
	}

	// Node Identity
	if cfg.Alias != "" {
		conf.Alias = cfg.Alias
	}
	if cfg.Color != "" {
		conf.Color = cfg.Color
	}

	// Watchtower
	if cfg.WatchtowerActive {
		conf.Watchtower.Active = cfg.WatchtowerActive
		if cfg.WatchtowerDir != "" {
			conf.Watchtower.TowerDir = cfg.WatchtowerDir
		}
	}

	// Public Node Configuration
	conf.RawExternalIPs = append(conf.RawExternalIPs, cfg.ExternalIPs...)
	conf.ExternalHosts = append(conf.ExternalHosts, cfg.ExternalHosts...)
	if cfg.DisableListen {
		conf.DisableListen = cfg.DisableListen
	}
	if cfg.NAT {
		conf.NAT = cfg.NAT
	}

	// Routing & Forwarding
	if cfg.MinHTLC > 0 {
		conf.Flokicoin.MinHTLCIn = lnwire.MilliLoki(cfg.MinHTLC)
	}
	if cfg.BaseFee > 0 {
		conf.Flokicoin.BaseFee = lnwire.MilliLoki(cfg.BaseFee)
	}
	if cfg.FeeRate > 0 {
		conf.Flokicoin.FeeRate = lnwire.MilliLoki(cfg.FeeRate)
	}
	if cfg.TimeLockDelta > 0 {
		conf.Flokicoin.TimeLockDelta = cfg.TimeLockDelta
	}
	if cfg.AcceptKeySend {
		conf.AcceptKeySend = cfg.AcceptKeySend
	}
	if cfg.AcceptAMP {
		conf.AcceptAMP = cfg.AcceptAMP
	}
	if cfg.Wumbo {
		conf.ProtocolOptions.WumboChans = true
	}

	// Network Graph & Gossip
	if cfg.NumGraphSyncPeers > 0 {
		conf.NumGraphSyncPeers = cfg.NumGraphSyncPeers
	}
	if cfg.HistoricalSyncInterval > 0 {
		conf.HistoricalSyncInterval = cfg.HistoricalSyncInterval
	}
	if cfg.IgnoreHistoricalFilters {
		conf.IgnoreHistoricalGossipFilters = cfg.IgnoreHistoricalFilters
	}
	if cfg.RejectHTLC {
		conf.RejectHTLC = cfg.RejectHTLC
	}
	if cfg.StaggerInitialReconnect {
		conf.StaggerInitialReconnect = cfg.StaggerInitialReconnect
	}
	if cfg.MaxOutgoingCltvExpiry > 0 {
		conf.MaxOutgoingCltvExpiry = cfg.MaxOutgoingCltvExpiry
	}

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
