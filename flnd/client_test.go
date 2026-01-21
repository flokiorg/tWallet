package flnd

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"net"
	"os"
	"testing"
	"time"

	"github.com/flokiorg/flnd"
	"github.com/flokiorg/flnd/signal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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

// Connection details for tests (can be overridden by environment or manually)
var (
	testAddress  = "lab.in.ionance.com:10005"
	testMacaroon = "0201036c6e6402f801030a105ed03bdbd9510d8289834908a76642841201301a160a0761646472657373120472656164120577726974651a130a04696e666f120472656164120577726974651a170a08696e766f69636573120472656164120577726974651a210a086d616361726f6f6e120867656e6572617465120472656164120577726974651a160a076d657373616765120472656164120577726974651a170a086f6666636861696e120472656164120577726974651a160a076f6e636861696e120472656164120577726974651a140a057065657273120472656164120577726974651a180a067369676e6572120867656e6572617465120472656164000006208bb35d64a7eb62bdfbd5504a45a530cf148d22f4b7101cf20f755dc24b041218"
)

func TestFetchTransactions(t *testing.T) {
	if testAddress == "" || testMacaroon == "" {
		t.Skip("testAddress and testMacaroon must be set")
	}

	// Connect to the node
	// Note: We use InsecureSkipVerify to solve the EOF error (Server expects TLS).
	creds := credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})
	// Check if host is valid, otherwise dial might hang
	if _, _, err := net.SplitHostPort(testAddress); err != nil {
		t.Fatalf("invalid address format: %v", err)
	}
	conn, err := grpc.NewClient(testAddress, grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatalf("failed to connect to %s: %v", testAddress, err)
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize the client
	c := NewClient(ctx, conn, &flnd.Config{})

	// Handle macaroon
	macBytes, err := hex.DecodeString(testMacaroon)
	if err != nil {
		t.Fatalf("failed to decode macaroon: %v", err)
	}
	tmpFile, err := os.CreateTemp("", "macaroon")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write(macBytes); err != nil {
		t.Fatalf("failed to write macaroon: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}
	if err := c.LoadMacaroon(tmpFile.Name()); err != nil {
		t.Fatalf("failed to load macaroon: %v", err)
	}

	// Fetch transactions
	txs, err := c.FetchTransactions()
	if err != nil {
		t.Fatalf("failed to fetch transactions: %v", err)
	}

	t.Logf("Fetched %d transactions", len(txs))
	// Log last 5 for verification
	start := 0
	if len(txs) > 5 {
		start = len(txs) - 5
	}
	for i := start; i < len(txs); i++ {
		tx := txs[i]
		t.Logf("Tx %d: %s (amount: %d, time: %s)", i, tx.TxHash, tx.Amount, time.Unix(tx.TimeStamp, 0).Format(time.RFC3339))
	}
}

func TestFetchTransactionsWithProgress(t *testing.T) {
	if testAddress == "" || testMacaroon == "" {
		t.Skip("testAddress and testMacaroon must be set")
	}

	// 1. Setup connection (Duplicate setup code for isolation)
	creds := credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})
	conn, err := grpc.NewClient(testAddress, grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := NewClient(ctx, conn, &flnd.Config{})

	// 2. Load Macaroon
	macBytes, err := hex.DecodeString(testMacaroon)
	if err != nil {
		t.Fatal(err)
	}
	tmpFile, _ := os.CreateTemp("", "macaroon_prog")
	defer os.Remove(tmpFile.Name())
	tmpFile.Write(macBytes)
	tmpFile.Close()
	if err := c.LoadMacaroon(tmpFile.Name()); err != nil {
		t.Fatal(err)
	}

	// 3. Define progress callback
	callCount := 0
	lastCount := 0
	progressCallback := func(count int) {
		callCount++
		t.Logf("Progress update #%d: %d transactions loaded", callCount, count)
		if count < lastCount {
			t.Errorf("Progress count should be increasing, got %d < %d", count, lastCount)
		}
		lastCount = count
	}

	// 4. Fetch with options
	opts := FetchTransactionsOptions{
		OnProgress: progressCallback,
	}

	txs, err := c.FetchTransactionsWithOptions(opts)
	if err != nil {
		t.Fatalf("failed to fetch with progress: %v", err)
	}

	t.Logf("Total transactions fetched: %d", len(txs))
	if callCount == 0 && len(txs) > 0 {
		t.Error("OnProgress was never called despite having transactions")
	}
	if lastCount != len(txs) {
		t.Logf("Warning: last progress count (%d) != total result count (%d). This might happen if Deduplication reduced the count after the last progress update.", lastCount, len(txs))
	}
}
