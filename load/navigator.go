// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package load

import (
	"github.com/rivo/tview"
)

type Navigator struct {
	*tview.Application
	pages *tview.Pages
}

func NewNavigator(app *tview.Application, pages *tview.Pages) *Navigator {
	return &Navigator{
		Application: app,
		pages:       pages,
	}
}

func (n *Navigator) NavigateTo(page tview.Primitive) {
	n.pages.HidePage("main").AddAndSwitchToPage("main", page, true)
}

func (n *Navigator) ShowModal(modal tview.Primitive) {
	n.pages.RemovePage("dialog").AddPage("dialog", modal, true, true)
}

func (n *Navigator) CloseModal() {
	n.pages.RemovePage("dialog")
}
