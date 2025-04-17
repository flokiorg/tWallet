// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package shared

import (
	"github.com/flokiorg/go-flokicoin/chainutil"
	"github.com/flokiorg/go-flokicoin/wire"
	"github.com/flokiorg/walletd/waddrmgr"
	"github.com/flokiorg/walletd/wallet"
	"github.com/flokiorg/walletd/walletmgr"
)

type MnemonicLen int

const (
	W12 MnemonicLen = 12
	W18 MnemonicLen = 18
	W24 MnemonicLen = 24
)

func IsValidMnemonicLen(c MnemonicLen) bool {
	switch c {
	case W12, W18, W24:
		return true
	default:
		return false
	}
}

type SeedType uint8

const (
	MNEMONIC SeedType = iota
	HEX
)

type EntropyLen uint8

const (
	ENTROPY_LENGTH_12 EntropyLen = 16
	ENTROPY_LENGTH_18 EntropyLen = 24
	ENTROPY_LENGTH_24 EntropyLen = 32
)

type Wallet interface {
	Create(seedLen uint8, name, passphrase string) (string, []string, error)
	RestoreByHex(hexData string, name, passphrase string) (privKeyHex string, words []string, err error)
	RestoreByMnemonic(input []string, name, passphrase string) (privKeyHex string, words []string, err error)

	IsOpened() bool
	IsSynced() bool
	Synchronize() (*waddrmgr.BlockStamp, error)
	Balance() float64
	Watch() (<-chan *wallet.AccountNotification, <-chan *wallet.TransactionNotifications, <-chan *wallet.SpentnessNotifications, chan interface{}, chan error)
	ChangePrivatePassphrase(old, new []byte) error
	SimpleTransfer(privPass []byte, address chainutil.Address, amount chainutil.Amount, feePerByte chainutil.Amount) (*wire.MsgTx, error)
	SimpleTransferFee(address chainutil.Address, amount chainutil.Amount, feePerByte chainutil.Amount) (*chainutil.Amount, error)
	GetNextAddress() (chainutil.Address, error)
	GetLastAddress() (chainutil.Address, error)
	FetchTransactions() ([]walletmgr.TransactionServiceResult, error)
	Recover(chan<- uint32) (*waddrmgr.BlockStamp, error)
	DestroyWallet()
	CurrentWalletBlock() (*waddrmgr.BlockStamp, error)
	CurrentBestBlock() (*waddrmgr.BlockStamp, error)
}
