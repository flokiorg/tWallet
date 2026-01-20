// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

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
	defaultMainnetFeeURL     = "https://lokichain.info/api/v1/fees/recommended"

	defaultTransactionDisplayLimit = 121

	defaultRPCListener  = "127.0.0.1:10005"
	defaultRESTListener = "127.0.0.1:5050"
	defaultRestCORS     = "http://localhost:3000"
	defaultPeerListener = "0.0.0.0:5521"

	parser *flags.Parser
)

type cliOptions struct {
	config.AppConfig
}

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
}

func main() {

	var opts cliOptions

	parser = flags.NewParser(&opts, flags.Default|flags.PassDoubleDash)
	parser.SubcommandsOptional = true
	if _, err := parser.Parse(); err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		log.Fatal().Err(err).Msg("failed to parse command line")
	}

	if opts.Version {
		fmt.Println("Version:", Version)
		return
	}

	fmt.Println(ArtOrange + ArtBright + ArtText + "\nv" + Version + "\n" + ArtReset)

	defaultConfigPath, err := GetFullPath(defaultConfigFilename)
	if err != nil {
		showHelpAndExit("failed to resolve default config path", err)
	}
	if opt := parser.FindOptionByShortName('c'); !optionDefined(opt) && FileExists(defaultConfigPath) {
		opts.ConfigFile = defaultConfigPath
	}

	if opts.ConfigFile != "" {
		err := flags.NewIniParser(parser).ParseFile(opts.ConfigFile)
		if err != nil {
			showHelpAndExit("failed to parse configuration file", err)
		}
	}

	if opt := parser.FindOptionByShortName('t'); !optionDefined(opt) {
		opts.ConnectionTimeout = defaultConnectionTimeout
	}

	if opt := parser.FindOptionByShortName('w'); !optionDefined(opt) {
		opts.Walletdir = chainutil.AppDataDir(defaultAppDataDir, false)
	}

	if opts.TransactionDisplayLimit <= 0 {
		opts.TransactionDisplayLimit = defaultTransactionDisplayLimit
	}

	opts.Network = defaultNetwork
	if opts.RegressionTest {
		opts.Network = &chaincfg.RegressionNetParams
	} else if opts.Testnet {
		opts.Network = &chaincfg.TestNet3Params
	}

	if opt := parser.FindOptionByLongName("feeurl"); !optionDefined(opt) && opts.Network.Name == chaincfg.MainNetParams.Name {
		opts.Feeurl = defaultMainnetFeeURL
	}

	// Security Hardening: Set secure defaults if not configured
	if len(opts.RawRPCListeners) == 0 {
		opts.RawRPCListeners = []string{defaultRPCListener}
	}
	if len(opts.RawRESTListeners) == 0 {
		opts.RawRESTListeners = []string{defaultRESTListener}
	}
	if len(opts.RestCORS) == 0 {
		opts.RestCORS = []string{defaultRestCORS}
	}
	if len(opts.RawListeners) == 0 {
		opts.RawListeners = []string{defaultPeerListener}
	}
	if opt := parser.FindOptionByLongName("tlsautorefresh"); !optionDefined(opt) {
		opts.TLSAutoRefresh = true
	}

	if opt := parser.FindOptionByLongName("protocol.option-zeroconf"); !optionDefined(opt) {
		opts.ProtocolOptionZeroConf = true
	}
	if opt := parser.FindOptionByLongName("protocol.option-scid-alias"); !optionDefined(opt) {
		opts.ProtocolOptionScidAlias = true
	}

	usedType, unusedType, err := GetAddressTypesFromName(opts.AddressType)
	if err != nil {
		showHelpAndExit("invalid address type", err)
	}
	opts.UsedAddressType = usedType
	opts.UnusedAddressType = unusedType

	logLevel := shared.ParseLogLevel(opts.LogLevel)
	logPath := filepath.Join(opts.Walletdir, "twallet.log")
	log.Logger = shared.CreateFileLogger(logPath, logLevel)
	fmt.Printf("Starting twallet (network=%s, wallet_dir=%s)\n",
		opts.Network.Name, opts.Walletdir)

	origAutoRecover := os.Getenv("TWALLET_AUTO_RECOVER")
	restartForRecovery := false
	for {
		if restartForRecovery {
			_ = os.Setenv("TWALLET_AUTO_RECOVER", "1")
		} else {
			_ = os.Setenv("TWALLET_AUTO_RECOVER", origAutoRecover)
		}

		app := tui.NewApp(&opts.AppConfig)

		func() {
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
		}()

		fmt.Println("Shutting down...")
		app.Close()
		fmt.Println("Shutdown complete")

		if app.ShouldRestartForRecovery() {
			restartForRecovery = true
			continue
		}

		break
	}

	_ = os.Setenv("TWALLET_AUTO_RECOVER", origAutoRecover)
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
