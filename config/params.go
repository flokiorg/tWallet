package config

import (
	"github.com/flokiorg/flnd/lnrpc"
	"github.com/flokiorg/twallet/flnd"
)

type AppConfig struct {
	flnd.ServiceConfig
	ConfigFile      string `short:"c" long:"config" description:"Path to configuration file"`
	LogLevel        string `long:"loglevel" choice:"trace" choice:"debug" choice:"info" choice:"warn" choice:"error" choice:"fatal" choice:"panic" default:"info" description:"Logging level for twallet output"`
	DefaultPassword string `long:"defaultpassword" description:"Use default passphrase for locking (TESTING ONLY, DO NOT USE IN MAINNET OR PRODUCTION ENVIRONMENTS)"`
	AddressType     string `long:"addresstype" choice:"taproot" choice:"segwit" choice:"nested-segwit" default:"segwit" description:"Address type to generate (taproot, segwit, or nested-segwit)."`
	AutoUnlock      bool   `long:"autounlock" description:"Automatically unlock the wallet on startup using defaultpassword (WARNING: Use with caution)"`
	Version         bool   `short:"v" description:"Print version"`

	AutoRefreshInterval int `long:"autorefreshinterval" description:"Interval in seconds to automatically refresh the TUI (0 to disable)" default:"300"`

	UsedAddressType   lnrpc.AddressType
	UnusedAddressType lnrpc.AddressType
}
