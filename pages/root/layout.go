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
	load   *load.Load
}

func NewLayout(l *load.Load, page tview.Primitive) tview.Primitive {

	if currentLayout != nil {
		currentLayout.destroy()
	}

	layout := &Layout{
		Flex: tview.NewFlex(),
		load: l,
	}

	layout.header = NewHeader(l)
	layout.body = NewBody(page)
	layout.footer = NewFooter(l)

	layout.SetDirection(tview.FlexRow).
		AddItem(layout.header, 6, 0, false).
		AddItem(layout.body, 0, 1, true).
		AddItem(layout.footer, 2, 0, false)

	layout.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if layout.body != nil && l.Application.GetFocus() != layout.body {
			l.Application.SetFocus(layout.body) // Restore focus to body
		}
		return event
	})

	currentLayout = layout
	return currentLayout
}

func (l *Layout) destroy() {
	if currentLayout.header != nil {
		currentLayout.header.Destroy()
	}
	if currentLayout.footer != nil {
		currentLayout.footer.Destroy()
	}
}
