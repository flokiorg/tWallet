// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package pages

import (
	"github.com/rivo/tview"

	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/pages/onboard"
	"github.com/flokiorg/twallet/pages/root"
	"github.com/flokiorg/twallet/pages/wallet"
)

func NewEntrypoint(l *load.Load) *root.Layout {

	var page tview.Primitive
	if l.Wallet.IsOpened() {
		page = wallet.NewPage(l)
	} else {
		page = onboard.NewPage(l)
	}

	layout := root.NewLayout(l, page)

	if l.Wallet.IsOpened() && !l.Wallet.IsSynced() {
		go l.StartSync()
	}

	return layout
}
