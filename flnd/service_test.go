package flnd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/flokiorg/flnd/lnrpc"
	"github.com/flokiorg/go-flokicoin/chaincfg"
)

func createService(t *testing.T, flnDir string) *Service {

	svc := New(context.Background(), &ServiceConfig{
		Walletdir: flnDir,
		Network:   &chaincfg.TestNet3Params,
	})

	t.Cleanup(svc.Stop)

	return svc
}

func createWallet(t *testing.T, svc *Service) {

	sub := svc.Subscribe()
	defer svc.Unsubscribe(sub)

loop:
	for update := range sub {
		switch update.State {
		case StatusNoWallet:
			break loop

		case StatusInit:
			continue

		default:
			t.Fatalf("unexpected state got:%v want:%v", update.State, StatusNoWallet)

		}
	}

	mhex, mnemonic, err := svc.CreateWallet(walletPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("mhex: %v", mhex)
	t.Logf("mnemonic: %v", mnemonic)

	for {
		select {
		case update := <-sub:
			t.Logf("state: %v", update.State)
			if update.State == StatusReady {
				return
			}
		case <-time.After(time.Second * 10):
			t.Fatal("wallet creation: timeout")
		}
	}
}

func openWallet(t *testing.T, flnDir string) *Service {

	svc := createService(t, flnDir)
	sub := svc.Subscribe()
	defer svc.Unsubscribe(sub)

	for update := range sub {
		switch update.State {
		case StatusLocked:
			return svc

		case StatusInit:
			continue

		default:
			t.Fatalf("unexpected state got:%v want:%v", update.State, StatusLocked)

		}
	}

	t.Fatalf("unexpected state  want:%v", StatusLocked)

	return svc
}

func walletExists(t *testing.T, svc *Service) {
	exists, err := svc.WalletExists()
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected wallet to exist ")
	}
}

func walletStarted(t *testing.T, svc *Service) {
	sub := svc.Subscribe()
	defer svc.Unsubscribe(sub)
	states := []string{}
	for {
		select {
		case update := <-sub:
			// t.Logf("state: %v", update.State)
			switch update.State {
			case StatusReady, StatusNone, StatusNoWallet, StatusLocked:
				return
			}
			states = append(states, string(update.State))
		case <-time.After(time.Second * 10):
			t.Fatalf("wallet started: timeout. got: %v", states)
		}
	}
}

func walletReady(t *testing.T, svc *Service) {
	sub := svc.Subscribe()
	defer svc.Unsubscribe(sub)
	states := []string{}
	for {
		select {
		case update := <-sub:
			// t.Logf("state: %v", update.State)
			switch update.State {
			case StatusReady:
				return
			}
			states = append(states, string(update.State))
		case <-time.After(time.Second * 10):
			t.Fatalf("wallet ready: timeout. got: %v", states)
		}
	}
}

func TestServiceConnection(t *testing.T) {
	svc := createService(t, createTestTempDir(t))
	defer svc.Stop()
	walletStarted(t, svc)
}

func TestServiceCreateWallet(t *testing.T) {
	svc := createService(t, createTestTempDir(t))
	defer svc.Stop()
	createWallet(t, svc)
}

func TestServiceBalance(t *testing.T) {
	svc := createService(t, createTestTempDir(t))
	defer svc.Stop()
	createWallet(t, svc)
	balance, err := svc.Balance()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("balance: %v", balance)
}

func TestServiceMulticonnect(t *testing.T) {
	svc := createService(t, createTestTempDir(t))
	createWallet(t, svc)
	isLocked, err := svc.IsLocked()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("isLocked: %v", isLocked)
	balance, err := svc.Balance()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("balance: %v", balance)
	svc.Stop()
	t.Log("stoped")

	svc = openWallet(t, svc.flndConfig.LndDir)
	if err := svc.Unlock(walletPassphrase); err != nil {
		t.Fatal(err)
	}
	walletReady(t, svc)
	balance, err = svc.Balance()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("balance: %v", balance)
	svc.Stop()
	t.Log("stoped")

	/////
	svc = openWallet(t, svc.flndConfig.LndDir)
	if err := svc.Unlock(walletPassphrase); err != nil {
		t.Fatal(err)
	}
	walletReady(t, svc)
	balance, err = svc.Balance()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("balance: %v", balance)
	svc.Stop()
	t.Log("stoped")

}

func TestServiceLocks(t *testing.T) {
	svc := createService(t, createTestTempDir(t))
	createWallet(t, svc)
	isLocked, err := svc.IsLocked()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("isLocked: %v", isLocked)
	svc.Stop()
	t.Log("stoped")

	_, err = svc.IsLocked()
	if !errors.Is(err, ErrDaemonNotRunning) {
		t.Fatal(err)
	}

	/////
	svc = openWallet(t, svc.flndConfig.LndDir)
	if err := svc.Unlock(walletPassphrase); err != nil {
		t.Fatal(err)
	}
	walletReady(t, svc)
	balance, err := svc.Balance()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("balance: %v", balance)
	svc.Stop()
	t.Log("stoped")

	///
	svc = openWallet(t, svc.flndConfig.LndDir)
	if err := svc.Unlock(walletPassphrase); err != nil {
		t.Fatal(err)
	}
	walletReady(t, svc)
	balance, err = svc.Balance()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("balance: %v", balance)
	svc.Stop()
	t.Log("stoped")
}

func TestServiceWalletExists(t *testing.T) {

	svc := createService(t, createTestTempDir(t))

	walletStarted(t, svc)

	exists, err := svc.WalletExists()
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("expected wallet to not exist ")
	}

	createWallet(t, svc)
	walletReady(t, svc)
	walletExists(t, svc)

	svc.Stop()
	t.Log("stoped")

}

func TestServiceWalletManager(t *testing.T) {
	svc := createService(t, createTestTempDir(t))

	createWallet(t, svc)

	txs, err := svc.FetchTransactions()
	if err != nil {
		t.Fatal(err)
	}

	if len(txs) != 0 {
		t.Fatalf("unexpected transactions length got:%v want:%v", len(txs), 0)
	}

	for _, at := range []lnrpc.AddressType{lnrpc.AddressType_UNUSED_NESTED_PUBKEY_HASH, lnrpc.AddressType_UNUSED_WITNESS_PUBKEY_HASH, lnrpc.AddressType_UNUSED_TAPROOT_PUBKEY} {
		address, err := svc.GetNextAddress(at)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("address[%v]: %v", at, address)
	}

	balance, err := svc.Balance()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("balance: %v", balance)

	isLocked, err := svc.IsLocked()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("isLocked: %v", isLocked)

	svc.Stop()
	t.Log("stoped")
}

func TestRestoreWalletFromMnemonic(t *testing.T) {
	svc := createService(t, createTestTempDir(t))
	walletStarted(t, svc)

	expectedAddress := "2NBZPT4F5Qt5vCaSBP4a5F52T55ckzSFM3r"
	expectedEncipheredSeed := "00d4d7fa50ca971b409719f378fa888bc463f0ecf1517293afffbf3757c7f8a874"

	mnemonic := []string{
		"absorb", "pluck", "write", "pave", "practice", "misery",
		"across", "tobacco", "vibrant", "sick", "peasant", "bleak",
		"economy", "wear", "record", "clay", "income", "output",
		"zoo", "lazy", "install", "token", "tired", "attend",
	}

	encipheredSeed, err := svc.RestoreByMnemonic(mnemonic, walletPassphrase)
	if err != nil {
		t.Fatal(err)
	}

	if encipheredSeed != expectedEncipheredSeed {
		t.Fatalf("unexpected encipheredSeed, got:%v want:%v", encipheredSeed, expectedEncipheredSeed)

	}

	walletReady(t, svc)

	address, err := svc.GetNextAddress(lnrpc.AddressType_UNUSED_NESTED_PUBKEY_HASH)
	if err != nil {
		t.Fatal(err)
	}

	if address.String() != expectedAddress {
		t.Fatalf("unexpected address, got:%v want:%v", address.String(), expectedAddress)
	}

	svc.Stop()
	t.Log("stoped")
}

func TestAARestoreWalletFromEncipheredSeed(t *testing.T) {
	svc := createService(t, createTestTempDir(t))
	walletStarted(t, svc)

	expectedAddress := "2NBZPT4F5Qt5vCaSBP4a5F52T55ckzSFM3r"
	expectedMnemonics := strings.Join([]string{
		"absorb", "pluck", "write", "pave", "practice", "misery",
		"across", "tobacco", "vibrant", "sick", "peasant", "bleak",
		"economy", "wear", "record", "clay", "income", "output",
		"zoo", "lazy", "install", "token", "tired", "attend",
	}, " ")

	encipheredSeed := "00d4d7fa50ca971b409719f378fa888bc463f0ecf1517293afffbf3757c7f8a874"

	mnemonics, err := svc.RestoreByEncipheredSeed(encipheredSeed, walletPassphrase)
	if err != nil {
		t.Fatal(err)
	}

	if expectedMnemonics != strings.Join(mnemonics, " ") {

	}

	walletReady(t, svc)

	address, err := svc.GetNextAddress(lnrpc.AddressType_UNUSED_NESTED_PUBKEY_HASH)
	if err != nil {
		t.Fatal(err)
	}

	if address.String() != expectedAddress {
		t.Fatalf("unexpected address, got:%v want:%v", address.String(), expectedAddress)
	}

	svc.Stop()
	t.Log("stoped")

}

func TestAAChangePassphrase(t *testing.T) {
	svc := createService(t, createTestTempDir(t))
	createWallet(t, svc)
	walletReady(t, svc)
	svc.Stop()
	t.Log("stoped")

	newPassphrase := "newPassePhrase"
	svc = openWallet(t, svc.flndConfig.LndDir)
	if err := svc.ChangePassphrase(walletPassphrase, newPassphrase); err != nil {
		t.Fatal(err)
	}
	walletReady(t, svc)
	svc.Stop()
	t.Log("stoped")

	svc = openWallet(t, svc.flndConfig.LndDir)
	if err := svc.Unlock(walletPassphrase); err == nil {
		t.Fatal("error expected")
	}
	if err := svc.Unlock(newPassphrase); err != nil {
		t.Fatal(err)
	}

	walletReady(t, svc)

	_, err := svc.GetNextAddress(lnrpc.AddressType_UNUSED_NESTED_PUBKEY_HASH)
	if err != nil {
		t.Fatal(err)
	}

	svc.Stop()
	t.Log("stoped")
}
