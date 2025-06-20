// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/flokiorg/go-flokicoin/chaincfg"
	"github.com/flokiorg/go-flokicoin/chainutil"
	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/tui"
	"github.com/flokiorg/twallet/utils"
	"github.com/flokiorg/walletd/waddrmgr"
	"github.com/flokiorg/walletd/walletdb/bdb"
	"github.com/jessevdk/go-flags"
	bolterr "go.etcd.io/bbolt/errors"

	"github.com/flokiorg/walletd/walletmgr"
	"github.com/flokiorg/walletd/walletseed/bip39"
	"github.com/flokiorg/walletd/walletseed/bip39/wordlists"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	defaultDBTimeout             = 10 * time.Second
	defaultPubPass               = "/flc/public"
	defaultWordList              = wordlists.English
	defaultNetwork               = &chaincfg.MainNetParams
	defaultAppName               = "twallet"
	defaultElectrumPort          = 50001
	defaultConfigFilename        = "twallet.conf"
	defaultAccountID      uint32 = 1

	parser *flags.Parser
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
}

func main() {

	var cfg load.AppConfig

	parser = flags.NewParser(&cfg, flags.Default|flags.PassDoubleDash)
	if _, err := parser.Parse(); err != nil {
		os.Exit(1)
	}

	if cfg.Version {
		fmt.Println("Version:", utils.Version)
		return
	}

	defaultConfigPath, err := utils.GetFullPath(defaultConfigFilename)
	if err != nil {
		exitWithError("unexpected error", err)
	}
	if opt := parser.FindOptionByShortName('c'); !optionDefined(opt) && utils.FileExists(defaultConfigPath) {
		cfg.ConfigFile = defaultConfigPath
	}

	if cfg.ConfigFile != "" {
		err := flags.NewIniParser(parser).ParseFile(cfg.ConfigFile)
		if err != nil {
			exitWithError("Failed to parse configuration file", err)
		}
	}

	if opt := parser.FindOptionByShortName('e'); !optionDefined(opt) {
		exitWithError("Electrum server (-e, --electserver) is required but not provided.", nil)
	}
	electrumServerEndpoint, err := utils.ValidateAndNormalizeURI(cfg.ElectrumServer, defaultElectrumPort)
	if err != nil {
		exitWithError("invalid electserver", nil)
	}

	if opt := parser.FindOptionByShortName('t'); !optionDefined(opt) {
		cfg.DBTimeout = defaultDBTimeout
	}

	if opt := parser.FindOptionByShortName('d'); !optionDefined(opt) {
		cfg.WalletDir = chainutil.AppDataDir(defaultAppName, false)
	}

	if opt := parser.FindOptionByShortName('a'); !optionDefined(opt) {
		cfg.AccountID = defaultAccountID
	}

	network := defaultNetwork
	if cfg.RegressionTest {
		network = &chaincfg.RegressionNetParams
	} else if cfg.Testnet {
		network = &chaincfg.TestNet3Params
	}

	params := &walletmgr.WalletParams{
		Network:        network,
		ElectrumServer: electrumServerEndpoint,
		Path:           cfg.WalletDir,
		Timeout:        cfg.DBTimeout,
		PublicPassword: defaultPubPass,
		AddressScope:   waddrmgr.KeyScopeBIP0044,
		AccountID:      cfg.AccountID,
	}

	// Register the backend database
	bdb.Register()

	// init word list
	bip39.SetWordList(defaultWordList)

	wallet := walletmgr.NewWalletService(params)

	if err := wallet.OpenWallet(); err != nil {
		if errors.Is(err, bolterr.ErrTimeout) {
			log.Fatal().Msgf("timeout occurred: the wallet may already be opened by another process")
		} else if !errors.Is(err, walletmgr.ErrWalletNotfound) {
			log.Fatal().Err(err).Msgf("unable to open existing wallet")
		}
	}

	if err = os.MkdirAll(cfg.WalletDir, 0755); err != nil {
		log.Fatal().Err(err).Msgf("Failed to create wallet directory: %s", cfg.WalletDir)
	}

	appInfo := load.NewAppInfo(&cfg, params)
	app := tui.NewApp(appInfo, wallet)
	if err := app.Run(); err != nil {
		log.Fatal().Err(err).Msg("app failed")
	}
}

func exitWithError(msg string, err error) {
	log.Error().Err(err).Msg(msg)
	fmt.Println()
	parser.WriteHelp(os.Stdout)
	os.Exit(1)
}

func optionDefined(opt *flags.Option) bool {
	return opt != nil && opt.IsSet()
}
