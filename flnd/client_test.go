package flnd

import (
	"context"
	"testing"
	"time"

	"github.com/flokiorg/flnd/signal"
)

var (
	walletPassphrase = "/wallet/password"
)

func openConnection(t *testing.T) *daemon {

	conf := createConfig(t)
	interceptor, err := signal.Intercept()
	if err != nil {
		t.Fatal(err)
	}

	d, err := newDaemon(context.Background(), &conf, interceptor)
	if err != nil {
		t.Fatal(err)
	}

	return d
}

func TestClientState(t *testing.T) {
	d := openConnection(t)
	defer d.stop()

	c, err := d.start()
	if err != nil {
		t.Fatal(err)
	}

	select {
	case h := <-c.Health():

		if h.State != StatusNoWallet {
			t.Fatalf("expect noWallet state got: %v", h.State)
		}
	case <-time.After(time.Second * 3):
		t.Fatal("health check failed")
	}
}

func TestClientWallet(t *testing.T) {
	d := openConnection(t)
	defer d.stop()

	c, err := d.start()
	if err != nil {
		t.Fatal(err)
	}

	select {
	case h := <-c.Health():
		if h.State != StatusNoWallet {
			t.Fatal("expect noWallet state")
		}
	case <-time.After(time.Second * 3):
		t.Fatal("health check failed")
	}

	mhex, mnemonic, err := c.Create(walletPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("mhex: %v", mhex)
	t.Logf("mnemonic: %v", mnemonic)

loop:
	for {
		select {
		case h := <-c.Health():
			t.Logf("state: %v", h.State)
			if h.State == StatusDown && h.Err != nil {
				t.Fatal(h.Err)
			} else if h.State == StatusReady {
				break loop
			}
		case <-time.After(time.Second * 180):
			t.Fatal("health check failed")
		}
	}

	balance, err := c.Balance()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("balance: %v", balance)
}
