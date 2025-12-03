package wallet

import (
	"context"

	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/shared"
)

func (w *Wallet) changePassword() {

	w.nav.ShowModal(components.NewDialog(
		"Confirm Action",
		"To change your password, the wallet must first be locked. Do you want to proceed?",
		w.nav.CloseModal,
		[]string{"Cancel", "Yes"},
		w.nav.CloseModal,
		func() {
			if w.busy {
				return
			}
			w.busy = true
			go func() {
				w.load.Notif.ShowToast("🔒 locking...")
				w.load.Wallet.Restart(context.Background())
				w.load.Application.QueueUpdateDraw(func() {
					w.load.Go(shared.CHANGE)
					w.busy = false
				})
			}()
		},
	))

}

func (w *Wallet) lockWallet() {

	w.nav.ShowModal(components.NewDialog(
		"Confirm Action",
		"Are you sure you want to lock the wallet?",
		w.nav.CloseModal,
		[]string{"Cancel", "Yes"},
		w.nav.CloseModal,
		func() {
			if w.busy {
				return
			}
			w.busy = true
			go func() {
				w.load.Notif.ShowToast("🔒 locking...")
				w.load.Wallet.Restart(context.Background())
				w.load.Application.QueueUpdateDraw(func() {
					w.load.Go(shared.LOCK)
					w.busy = false
				})
			}()
		},
	))
}
