// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package root

import (
	"errors"
	"fmt"

	"github.com/rivo/tview"

	"github.com/flokiorg/flnd/flnwallet"
	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/load"
	"github.com/gdamore/tcell/v2"
)

type Footer struct {
	*tview.Grid
	load       *load.Load
	status     *components.Circle
	statusText *tview.TextView
	infoText   *tview.TextView
	destroy    chan struct{}
}

func NewFooter(l *load.Load) *Footer {
	f := &Footer{
		Grid:       tview.NewGrid(),
		status:     components.NewCircle(),
		statusText: tview.NewTextView().SetTextAlign(tview.AlignRight).SetDynamicColors(true),
		infoText:   tview.NewTextView().SetTextAlign(tview.AlignCenter).SetDynamicColors(true),
		load:       l,
		destroy:    make(chan struct{}),
	}

	f.status.SetColor(components.YELLOW)
	f.statusText.SetBorderPadding(0, 0, 0, 2)

	leftSide := tview.NewTextView()
	leftSide.SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetBorderPadding(0, 0, 1, 1)

	switch ev := f.load.Wallet.GetLastEvent(); ev.State {
	case flnwallet.StatusLocked, flnwallet.StatusDown:
		break

	default:
		if ok, err := f.load.Wallet.IsLocked(); !ok && !errors.Is(err, flnwallet.ErrDaemonNotRunning) {
			fmt.Fprintf(leftSide, "[%s:-:b]<c> [white:-:-]Change Password ", tcell.ColorLightSkyBlue)
			fmt.Fprintf(leftSide, "[%s:-:b]<l> [white:-:-]Lock Wallet", tcell.ColorLightSkyBlue)
		}
	}

	f.SetRows(0).SetColumns(37, 0, 26, 3).
		AddItem(leftSide, 0, 0, 1, 1, 0, 0, false).
		AddItem(f.infoText, 0, 1, 1, 1, 0, 0, false).
		AddItem(f.statusText, 0, 2, 1, 1, 0, 0, false).
		AddItem(f.status, 0, 3, 1, 1, 0, 0, false)

	go f.updates()

	return f
}

func (f *Footer) updates() {

	for {
		select {

		case text := <-f.load.Notif.Toast():
			f.updateInfoText(text)

		case hs := <-f.load.Notif.Health():

			switch hs.Level {
			case load.HealthGreen:
				f.updateStatus(components.GREEN)
				f.updateStatusText(hs.Info)

			case load.HealthRed:
				f.load.Logger.Trace().Msgf("health-err: %v", hs.Info)
				f.updateStatus(components.RED)
				f.updateStatusText(hs.Info)
				if hs.Err != nil {
					f.updateInfoText(fmt.Sprintf("[red:-:-]Error: [-:-:-]%s", hs.Err.Error()))
				}

			case load.HealthOrange:
				f.load.Logger.Trace().Msgf("health-info: %v", hs.Info)
				f.updateStatus(components.YELLOW)
				f.updateStatusText(hs.Info)

			}

		case <-f.destroy:
			return
		}

	}
}

func (f *Footer) updateStatus(flagColor components.CircleColor) {
	f.load.Application.QueueUpdateDraw(func() {
		f.status.SetColor(flagColor)
	})
}

func (f *Footer) updateInfoText(notif string) {
	f.load.Application.QueueUpdateDraw(func() {
		f.infoText.SetText(notif)
	})
}

func (f *Footer) updateStatusText(notif string) {
	f.load.Application.QueueUpdateDraw(func() {
		f.statusText.SetText(notif)
	})
}

func (f *Footer) Destroy() {
	close(f.destroy)
}
