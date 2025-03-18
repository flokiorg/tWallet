// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package root

import (
	"errors"
	"fmt"

	"github.com/rivo/tview"

	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/walletd/chain/electrum"
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

	fmt.Fprintf(leftSide, "[%s:-:b]<c> [white:-:-]Change Password\t\t", tcell.ColorLightSkyBlue)
	// fmt.Fprintf(leftSide, "[%s:-:b]<b> [white:-:-]Backup Seed", tcell.ColorLightSkyBlue)

	f.SetRows(0).SetColumns(30, 0, 20, 3).
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

		case text := <-f.load.Notif.ElectrumToast():
			f.updateStatusText(text)

		case err := <-f.load.Notif.ElectrumHealth():

			if errors.Is(err, electrum.NerrHealthPong) {
				f.updateStatus(components.GREEN)
				f.updateHeight()

			} else if errors.Is(err, electrum.NerrHealthRestarting) {
				f.updateStatus(components.YELLOW)
			} else {
				f.updateStatus(components.RED)
				if errors.Is(err, electrum.ErrServerShutdown) {
					go f.load.Restart()
				}
			}
		case <-f.destroy:
			return
		}

	}
}

func (f *Footer) updateHeight() {

	block, err := f.load.Wallet.CurrentWalletBlock()
	var height int32
	if err == nil {
		height = block.Height
	}

	bestBlock, exists := f.load.AppInfo.GetStartupBlock()
	if !exists || bestBlock.Height > height {
		f.updateStatusText("Syncing...")
	} else {
		f.updateStatusText(fmt.Sprintf("Height: %d", height))
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
