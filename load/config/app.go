package config

import (
	"github.com/flokiorg/flnd/flnwallet"
	"github.com/flokiorg/flnd/lnrpc"
)

type AppConfig struct {
	flnwallet.ServiceConfig
	DefaultPassword string `long:"defaultpassword" description:"Use default passphrase for locking (TESTING ONLY, DO NOT USE IN MAINNET OR PRODUCTION ENVIRONMENTS)"`
	AddressType     string `long:"addresstype" choice:"taproot" choice:"segwit" choice:"nested-segwit" default:"segwit" description:"Address type to generate (taproot, segwit, or nested-segwit)."`
	Version         bool   `short:"v" description:"Print version"`

	UsedAddressType   lnrpc.AddressType
	UnusedAddressType lnrpc.AddressType
}
