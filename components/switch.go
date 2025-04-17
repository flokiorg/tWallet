// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package components

import (
	"log"

	"github.com/rivo/tview"

	"github.com/flokiorg/twallet/load"
	"github.com/gdamore/tcell/v2"
)

type Switch struct {
	*tview.Grid
	nav      *load.Navigator
	button1  *SwitchButton
	button2  *SwitchButton
	onSelect func(int)
}

func NewSwitch(nav *load.Navigator, label1, label2 string, selectedIndex int, onSelect func(selectedIndex int)) *Switch {

	if selectedIndex != 0 && selectedIndex != 1 {
		log.Fatal("unexpected index")
	}

	s := &Switch{
		Grid:     tview.NewGrid(),
		button1:  NewSwitchButton(0, label1, false),
		button2:  NewSwitchButton(1, label2, false),
		onSelect: onSelect,
		nav:      nav,
	}

	s.Grid.SetRows(1).SetColumns(5, 0, 5)
	s.Grid.AddItem(s.button1, 1, 1, 1, 1, 0, 0, true).
		AddItem(tview.NewBox(), 1, 2, 1, 1, 0, 0, false).
		AddItem(s.button2, 1, 3, 1, 1, 0, 0, false)

	keyCapture := func(active, inactive *SwitchButton) func(*tcell.EventKey) *tcell.EventKey {
		return func(ev *tcell.EventKey) *tcell.EventKey {
			if ev.Key() == tcell.KeyEnter || (ev.Key() == tcell.KeyRune && ev.Rune() == ' ') {
				s.update(active, inactive)
				return nil
			}
			return ev
		}
	}

	mouseCapture := func(active, inactive *SwitchButton) func(tview.MouseAction, *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		return func(a tview.MouseAction, e *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
			if a == tview.MouseLeftClick {
				s.update(active, inactive)
			}
			return a, e
		}
	}

	s.button1.SetInputCapture(keyCapture(s.button1, s.button2))
	s.button2.SetInputCapture(keyCapture(s.button2, s.button1))

	s.button1.SetMouseCapture(mouseCapture(s.button1, s.button2))
	s.button2.SetMouseCapture(mouseCapture(s.button2, s.button1))

	if selectedIndex == 0 {
		s.update(s.button1, s.button2)
	} else {
		s.update(s.button2, s.button1)
	}

	return s
}

func (s *Switch) update(active *SwitchButton, inactive *SwitchButton) {
	go func() {
		s.nav.Application.QueueUpdateDraw(func() {
			active.SetActive(true)
			inactive.SetActive(false)
			s.onSelect(active.ID)
		})
	}()
}
