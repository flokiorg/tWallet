// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"errors"

	"github.com/flokiorg/go-flokicoin/chaincfg"
	"github.com/flokiorg/go-flokicoin/chainutil"
	"github.com/flokiorg/twallet/config"
	"github.com/flokiorg/twallet/shared"
	"github.com/flokiorg/twallet/tui"
	. "github.com/flokiorg/twallet/utils"
	"github.com/jessevdk/go-flags"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	defaultConnectionTimeout = 60 * time.Second
	defaultNetwork           = &chaincfg.MainNetParams
	defaultAppDataDir        = "flnd"
	defaultConfigFilename    = "twallet.conf"
	defaultMainnetFeeURL     = "https://flokichain.info/api/v1/fees/recommended"

	defaultMaxTransactionsLimit = 1000

	parser *flags.Parser
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
}

func main() {

	var cfg config.AppConfig

	parser = flags.NewParser(&cfg, flags.Default|flags.PassDoubleDash)
	if _, err := parser.Parse(); err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		log.Fatal().Err(err).Msg("failed to parse command line")
	}

	if cfg.Version {
		fmt.Println("Version:", Version)
		return
	}

	fmt.Println(ArtOrange + ArtBright + ArtText + "\nv" + Version + "\n" + ArtReset)

	defaultConfigPath, err := GetFullPath(defaultConfigFilename)
	if err != nil {
		showHelpAndExit("failed to resolve default config path", err)
	}
	if opt := parser.FindOptionByShortName('c'); !optionDefined(opt) && FileExists(defaultConfigPath) {
		cfg.ConfigFile = defaultConfigPath
	}

	if cfg.ConfigFile != "" {
		err := flags.NewIniParser(parser).ParseFile(cfg.ConfigFile)
		if err != nil {
			showHelpAndExit("failed to parse configuration file", err)
		}
	}

	if opt := parser.FindOptionByShortName('t'); !optionDefined(opt) {
		cfg.ConnectionTimeout = defaultConnectionTimeout
	}

	if opt := parser.FindOptionByShortName('w'); !optionDefined(opt) {
		cfg.Walletdir = chainutil.AppDataDir(defaultAppDataDir, false)
	}

	if cfg.MaxTransactionsLimit <= 0 {
		cfg.MaxTransactionsLimit = defaultMaxTransactionsLimit
	}

	cfg.Network = defaultNetwork
	if cfg.RegressionTest {
		cfg.Network = &chaincfg.RegressionNetParams
	} else if cfg.Testnet {
		cfg.Network = &chaincfg.TestNet3Params
	}

	if opt := parser.FindOptionByLongName("feeurl"); !optionDefined(opt) && cfg.Network.Name == chaincfg.MainNetParams.Name {
		cfg.Feeurl = defaultMainnetFeeURL
	}

	usedType, unusedType, err := GetAddressTypesFromName(cfg.AddressType)
	if err != nil {
		showHelpAndExit("invalid address type", err)
	}
	cfg.UsedAddressType = usedType
	cfg.UnusedAddressType = unusedType

	logLevel := shared.ParseLogLevel(cfg.LogLevel)
	logPath := filepath.Join(cfg.Walletdir, "twallet.log")
	log.Logger = shared.CreateFileLogger(logPath, logLevel)
	log.Info().
		Str("network", cfg.Network.Name).
		Str("wallet_dir", cfg.Walletdir).
		Str("log_level", logLevel.String()).
		Msg("starting twallet")

	app := tui.NewApp(&cfg)

	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			app.Stop()
			app.Close()
			log.Error().Interface("panic", r).Bytes("stack", stack).Msg("unhandled panic")
			fmt.Fprintf(os.Stderr, "\npanic: %v\n%s", r, stack)
			os.Exit(1)
		}
	}()

	if err := app.Run(); err != nil {
		app.Stop()
		log.Fatal().Err(err).Msg("app failed")
	}
	fmt.Println("Shutting down...")
	app.Close()
	fmt.Println("Shutdown complete")
}

func showHelpAndExit(msg string, err error) {
	if msg != "" {
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", msg, err)
		} else {
			fmt.Fprintln(os.Stderr, msg)
		}
	}
	fmt.Fprintln(os.Stderr)
	if parser != nil {
		parser.WriteHelp(os.Stderr)
	}
	os.Exit(1)
}

func optionDefined(opt *flags.Option) bool {
	return opt != nil && opt.IsSet()
}
