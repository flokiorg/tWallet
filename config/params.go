package config

import (
	"github.com/flokiorg/flnd/flnwallet"
	"github.com/flokiorg/flnd/lnrpc"
)

type AppConfig struct {
	flnwallet.ServiceConfig
	ConfigFile      string `short:"c" long:"config" description:"Path to configuration file"`
	LogLevel        string `long:"loglevel" choice:"trace" choice:"debug" choice:"info" choice:"warn" choice:"error" choice:"fatal" choice:"panic" default:"info" description:"Logging level for twallet output"`
	DefaultPassword string `long:"defaultpassword" description:"Use default passphrase for locking (TESTING ONLY, DO NOT USE IN MAINNET OR PRODUCTION ENVIRONMENTS)"`
	AddressType     string `long:"addresstype" choice:"taproot" choice:"segwit" choice:"nested-segwit" default:"segwit" description:"Address type to generate (taproot, segwit, or nested-segwit)."`
	Version         bool   `short:"v" description:"Print version"`

	UsedAddressType   lnrpc.AddressType
	UnusedAddressType lnrpc.AddressType
}
