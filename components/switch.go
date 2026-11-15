// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package components

import (
	"log"

	"github.com/flokiorg/twallet/load"
	"github.com/rivo/tview"

	"github.com/gdamore/tcell/v2"
)

type SwitchStyle struct {
	ButtonWidth           int
	GapWidth              int
	SidePadding           int
	RowPadding            int
	BackgroundColor       tcell.Color
	ButtonBackgroundColor tcell.Color
	ActiveBorderColor     tcell.Color
	InactiveBorderColor   tcell.Color
	ActiveTextColor       tcell.Color
	InactiveTextColor     tcell.Color
}

func (s SwitchStyle) normalized() SwitchStyle {
	style := s
	if style.ButtonWidth <= 0 {
		style.ButtonWidth = 18
	}
	if style.GapWidth < 0 {
		style.GapWidth = 0
	}
	if style.SidePadding < 0 {
		style.SidePadding = 0
	}
	if style.RowPadding < 0 {
		style.RowPadding = 0
	}
	return style
}

func DefaultSwitchStyle() SwitchStyle {
	return SwitchStyle{
		ButtonWidth:           18,
		GapWidth:              2,
		SidePadding:           2,
		RowPadding:            1,
		BackgroundColor:       tcell.ColorDefault,
		ButtonBackgroundColor: tcell.ColorDefault,
		ActiveBorderColor:     tcell.ColorOrange,
		InactiveBorderColor:   tcell.ColorGray,
		ActiveTextColor:       tcell.ColorOrange,
		InactiveTextColor:     tcell.ColorWhite,
	}
}

type Switch struct {
	*tview.Grid
	nav      *load.Navigator
	button1  *SwitchButton
	button2  *SwitchButton
	onSelect func(int)
	style    SwitchStyle
}

func NewSwitch(nav *load.Navigator, label1, label2 string, selectedIndex int, onSelect func(selectedIndex int)) *Switch {
	return NewSwitchWithStyle(nav, label1, label2, selectedIndex, DefaultSwitchStyle(), onSelect)
}

func NewSwitchWithStyle(nav *load.Navigator, label1, label2 string, selectedIndex int, style SwitchStyle, onSelect func(selectedIndex int)) *Switch {
	if selectedIndex != 0 && selectedIndex != 1 {
		log.Fatal("unexpected index")
	}

	style = style.normalized()

	s := &Switch{
		Grid:     tview.NewGrid(),
		button1:  NewSwitchButton(0, label1, false),
		button2:  NewSwitchButton(1, label2, false),
		onSelect: onSelect,
		nav:      nav,
		style:    style,
	}

	columns := []int{style.SidePadding, style.ButtonWidth, style.GapWidth, style.ButtonWidth, style.SidePadding}
	rows := []int{style.RowPadding, 0, style.RowPadding}
	s.Grid.SetRows(rows...).SetColumns(columns...)

	padBox := func() *tview.Box {
		b := tview.NewBox()
		if style.BackgroundColor != tcell.ColorDefault {
			b.SetBackgroundColor(style.BackgroundColor)
		}
		return b
	}

	if style.BackgroundColor != tcell.ColorDefault {
		s.Grid.SetBackgroundColor(style.BackgroundColor)
	}

	if style.ButtonBackgroundColor != tcell.ColorDefault {
		s.button1.SetBackgroundColor(style.ButtonBackgroundColor)
		s.button2.SetBackgroundColor(style.ButtonBackgroundColor)
	}
	s.button1.SetColors(style.ActiveBorderColor, style.InactiveBorderColor, style.ActiveTextColor, style.InactiveTextColor)
	s.button2.SetColors(style.ActiveBorderColor, style.InactiveBorderColor, style.ActiveTextColor, style.InactiveTextColor)

	s.Grid.AddItem(padBox(), 0, 0, 1, len(columns), 0, 0, false)
	s.Grid.AddItem(s.button1, 1, 1, 1, 1, 0, 0, true)
	gapBox := padBox()
	s.Grid.AddItem(gapBox, 1, 2, 1, 1, 0, 0, false)
	s.Grid.AddItem(s.button2, 1, 3, 1, 1, 0, 0, false)
	s.Grid.AddItem(padBox(), 2, 0, 1, len(columns), 0, 0, false)

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
