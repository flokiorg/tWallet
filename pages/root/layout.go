// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package root

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/flokiorg/twallet/load"
)

var (
	currentLayout *Layout
)

type Layout struct {
	*tview.Flex
	header *Header
	body   *Body
	footer *Footer
}

func NewLayout(l *load.Load, page tview.Primitive) *Layout {

	if currentLayout != nil {
		if currentLayout.header != nil {
			currentLayout.header.Destroy()
		}
		if currentLayout.footer != nil {
			currentLayout.footer.Destroy()
		}
	}

	layout := &Layout{
		Flex: tview.NewFlex(),
	}

	layout.header = NewHeader(l)
	layout.body = NewBody(page)

	layout.SetDirection(tview.FlexRow).
		AddItem(layout.header, 6, 0, false).
		AddItem(layout.body, 0, 1, true)

	if l.Wallet.IsOpened() {
		layout.footer = NewFooter(l)
		layout.AddItem(layout.footer, 2, 0, false)
	}

	// Ensure the focus is always on the body
	layout.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if layout.body != nil && l.Application.GetFocus() != layout.body {
			l.Application.SetFocus(layout.body) // Restore focus to body
		}
		return event
	})

	currentLayout = layout
	return currentLayout
}
