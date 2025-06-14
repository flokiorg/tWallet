// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/flokiorg/go-flokicoin/chaincfg"
	"github.com/flokiorg/go-flokicoin/chainutil"
	"github.com/flokiorg/twallet/load/config"
	"github.com/flokiorg/twallet/tui"
	"github.com/flokiorg/twallet/utils"
	"github.com/jessevdk/go-flags"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	defaultConnectionTimeout        = 60 * time.Second
	defaultNetwork                  = &chaincfg.MainNetParams
	defaultAppDataDir               = "flnd"
	defaultConfigFilename           = "twallet.conf"
	defaultMainnetFeeURL            = "https://flokichain.info/api/v1/fees/recommended"
	defaultAccountID         uint32 = 1

	parser *flags.Parser
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
}

func main() {

	var cfg config.AppConfig

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

	if opt := parser.FindOptionByShortName('t'); !optionDefined(opt) {
		cfg.ConnectionTimeout = defaultConnectionTimeout
	}

	if opt := parser.FindOptionByShortName('w'); !optionDefined(opt) {
		cfg.Walletdir = chainutil.AppDataDir(defaultAppDataDir, false)
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

	usedType, unusedType, err := utils.GetAddressTypesFromName(cfg.AddressType)
	if err != nil {
		exitWithError("Failed to parse configuration file", err)
	}
	cfg.UsedAddressType = usedType
	cfg.UnusedAddressType = unusedType

	app := tui.NewApp(&cfg)

	if err := app.Run(); err != nil {
		log.Fatal().Err(err).Msg("app failed")
	}
	fmt.Println("Shutting down...")
	app.Close()
	fmt.Println("Shutdown complete")
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
