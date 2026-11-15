// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package root

import (
	"github.com/rivo/tview"
)

type Body struct {
	*tview.Flex
	layout tview.Primitive
}

func NewBody(layout tview.Primitive) *Body {
	b := &Body{
		Flex:   tview.NewFlex(),
		layout: layout,
	}

	b.AddItem(layout, 0, 1, true)

	return b
}
