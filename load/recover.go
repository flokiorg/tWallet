// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package load

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flokiorg/flnd/flnwallet"
	"github.com/flokiorg/flnd/lncfg"
	"github.com/flokiorg/twallet/config"
)

// WalletHealth describes the current availability of the wallet service.
type WalletHealth struct {
	Healthy bool
	State   flnwallet.Status
	Reason  string
}

// CheckWalletHealth waits for the wallet to reach a ready/locked state or
// surfaces the first error state encountered within the timeout window.
func CheckWalletHealth(ctx context.Context, svc *flnwallet.Service, timeout time.Duration) (WalletHealth, error) {
	sub := svc.Subscribe()
	defer svc.Unsubscribe(sub)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return WalletHealth{}, ctx.Err()

		case <-timer.C:
			return WalletHealth{Healthy: false, State: flnwallet.StatusNone, Reason: "wallet did not become ready before timeout"}, nil

		case update, ok := <-sub:
			if !ok || update == nil {
				return WalletHealth{Healthy: false, State: flnwallet.StatusDown, Reason: "wallet service closed unexpectedly"}, nil
			}

			switch update.State {
			case flnwallet.StatusReady, flnwallet.StatusUnlocked, flnwallet.StatusSyncing, flnwallet.StatusTransaction, flnwallet.StatusBlock:
				return WalletHealth{Healthy: true, State: update.State}, nil

			case flnwallet.StatusLocked:
				return WalletHealth{Healthy: true, State: update.State}, nil

			case flnwallet.StatusNoWallet:
				return WalletHealth{Healthy: false, State: update.State, Reason: "wallet not found"}, nil

			case flnwallet.StatusDown:
				reason := "wallet daemon reported down state"
				if update.Err != nil {
					reason = update.Err.Error()
				}
				return WalletHealth{Healthy: false, State: update.State, Reason: reason}, nil

			case flnwallet.StatusQuit:
				return WalletHealth{Healthy: false, State: update.State, Reason: "wallet service quit unexpectedly"}, nil

			case flnwallet.StatusNone, flnwallet.StatusInit:
				// Keep waiting
			}
		}
	}
}

// PurgeNeutrinoCache clears neutrino cache files for the configured network.
func PurgeNeutrinoCache(cfg *config.AppConfig, logf func(string)) error {
	if cfg == nil {
		return errors.New("missing app config for cache cleanup")
	}

	walletDir := strings.TrimSpace(cfg.Walletdir)
	if walletDir == "" {
		return errors.New("walletdir not configured; cannot locate neutrino cache")
	}

	network := "mainnet"
	if cfg.Network != nil && cfg.Network.Name != "" {
		network = cfg.Network.Name
	}
	network = lncfg.NormalizeNetwork(network)

	base := filepath.Join(walletDir, "data", "chain", "flokicoin", network)
	targets := []string{
		filepath.Join(base, "block_headers.bin"),
		filepath.Join(base, "reg_filter_headers.bin"),
		filepath.Join(base, "neutrino.db"),
		filepath.Join(base, "neutrino.sqlite"),
	}

	removed := false
	for _, path := range targets {
		err := os.Remove(path)
		switch {
		case err == nil:
			removed = true
			if logf != nil {
				logf(fmt.Sprintf("Removed %s", path))
			}
		case errors.Is(err, os.ErrNotExist):
			continue
		default:
			return fmt.Errorf("failed to remove %s: %w", path, err)
		}
	}

	if logf != nil {
		if removed {
			logf("Neutrino cache cleared.")
		} else {
			logf("No Neutrino cache files found to clear.")
		}
	}

	return nil
}
