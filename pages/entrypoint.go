// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package pages

import (
	"github.com/rivo/tview"

	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/pages/change"
	"github.com/flokiorg/twallet/pages/onboard"
	"github.com/flokiorg/twallet/pages/root"
	"github.com/flokiorg/twallet/pages/unlock"
	"github.com/flokiorg/twallet/pages/wallet"
	. "github.com/flokiorg/twallet/shared"
)

type Router struct {
	load *load.Load
}

func (r *Router) Go(p Page) {

	var layout tview.Primitive

	switch p {
	case WALLET:
		layout = root.NewLayout(r.load, wallet.NewPage(r.load))
	case LOCK:
		layout = root.NewLayout(r.load, unlock.NewPage(r.load, false))
	case ONBOARD:
		layout = root.NewLayout(r.load, onboard.NewPage(r.load))
	case CHANGE:
		layout = root.NewLayout(r.load, change.NewPage(r.load))
	}

	if layout != nil {
		if ev := r.load.Wallet.GetLastEvent(); ev != nil {
			r.load.Notif.ProcessEvent(ev)
		}
		r.load.Nav.NavigateTo(layout)
	}

}

func NewEntrypoint(l *load.Load) tview.Primitive {

	var page tview.Primitive
	exists, err := l.Wallet.WalletExists()
	if err != nil {
		l.Logger.Error().Err(err).Msg("failed")
	}

	switch exists {
	case true:
		page = unlock.NewPage(l, true)

	default:
		page = onboard.NewPage(l)
	}

	l.RegisterRouter(&Router{load: l})

	return root.NewLayout(l, page)
}
