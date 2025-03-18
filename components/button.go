// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package components

import (
	"fmt"
	"time"

	"github.com/rivo/tview"

	"github.com/gdamore/tcell/v2"
)

type SwitchButton struct {
	*tview.Grid
	ID            int
	Label         string
	TextView      *tview.TextView
	ActiveColor   tcell.Color
	InactiveColor tcell.Color
}

func NewSwitchButton(id int, label string, active bool) *SwitchButton {
	b := &SwitchButton{
		Grid:          tview.NewGrid(),
		TextView:      tview.NewTextView(),
		ID:            id,
		Label:         label,
		ActiveColor:   tcell.ColorOrange,
		InactiveColor: tcell.ColorGray,
	}
	b.TextView.SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetBorder(true)

	b.SetActive(active)

	b.SetRows(0, 3, 0).
		SetColumns(0).
		AddItem(b.TextView, 1, 0, 1, 1, 0, 0, true)
	return b
}

func (b *SwitchButton) SetActive(active bool) {
	if active {
		b.TextView.SetText(fmt.Sprintf("[orange::b]%s", b.Label)).
			SetBorderColor(b.ActiveColor)
	} else {
		b.TextView.SetText(fmt.Sprintf("[white::]%s", b.Label)).
			SetBorderColor(b.InactiveColor)
	}

}

type ConfirmButton struct {
	*tview.Grid
	label        string
	textView     *tview.TextView
	borderEffect bool
	isPressed    bool
	onClick      func()
	app          *tview.Application
}

// NewConfirmButton creates a new ConfirmButton instance with background color and border settings.
func NewConfirmButton(app *tview.Application, label string, borderEffect bool, bgColor tcell.Color, height int, onClick func()) *ConfirmButton {
	b := &ConfirmButton{
		Grid:         tview.NewGrid(),
		textView:     tview.NewTextView(),
		label:        label,
		borderEffect: borderEffect,
		isPressed:    false,
		app:          app,
		onClick:      onClick,
	}

	b.Grid.Box = tview.NewBox().SetBackgroundColor(bgColor)
	b.textView.Box = tview.NewBox().SetBackgroundColor(bgColor)

	b.textView.SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetBorder(borderEffect)

	b.SetRows(0, height, 0).
		SetColumns(0).
		AddItem(b.textView, 1, 0, 1, 1, 0, 0, false)

	// b.SetInputCapture(b.handleKeyEvents)
	b.SetMouseCapture(b.handleMouseEvents)
	b.render()

	return b
}

func (b *ConfirmButton) OnClick(callback func()) {
	b.onClick = callback
}

func (b *ConfirmButton) render() {
	if b.isPressed {
		if b.borderEffect {
			// Use border effect if the border is enabled
			b.textView.
				SetText(fmt.Sprintf("[%s::b]%s", tcell.ColorOrange, b.label)).
				SetBorderColor(tcell.ColorOrange)
		} else {
			// Use background effect if the border is disabled
			b.textView.
				SetText(fmt.Sprintf("[%s::b]%s", tcell.ColorGray, b.label)).
				SetBackgroundColor(tcell.ColorOrange)
			b.textView.SetBorder(true)
		}
	} else {
		if b.borderEffect {
			b.textView.
				SetText(fmt.Sprintf("[%s::b]%s", tcell.ColorOrange, b.label)).
				SetBorderColor(tcell.ColorOrange).Blur()
		} else {
			b.textView.
				SetText(fmt.Sprintf("[%s::b]%s", tcell.ColorGray, b.label)).
				SetBackgroundColor(tcell.ColorOrange).Blur()
			b.textView.SetBorder(false)

		}
	}
}

// // handleKeyEvents manages keyboard interactions.
// func (b *ConfirmButton) handleKeyEvents(event *tcell.EventKey) *tcell.EventKey {
// 	switch event.Key() {
// 	case tcell.KeyEnter, tcell.KeyRune:
// 		if event.Rune() == ' ' || event.Key() == tcell.KeyEnter {
// 			// Simulate press
// 			b.pressButton()
// 			// If Enter is pressed, trigger confirm action
// 			if b.onClick != nil {
// 				b.onClick()
// 			}
// 			return nil
// 		}
// 	}
// 	return event
// }

// handleMouseEvents manages mouse interactions.
func (b *ConfirmButton) handleMouseEvents(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {

	x, y := event.Position()

	// Get the component's boundaries.
	buttonX, buttonY, buttonWidth, buttonHeight := b.GetRect()

	// Check if the mouse click is within the component's boundaries.
	if x >= buttonX && x < buttonX+buttonWidth && y >= buttonY && y < buttonY+buttonHeight {
		// Check if it's a left-click.
		if action == tview.MouseLeftClick {
			b.pressButton()
			if b.onClick != nil {
				b.onClick()
			}
			return action, event
		}
	}

	return action, event

}

// pressButton changes the button's appearance to pressed and reverts it.
func (b *ConfirmButton) pressButton() {
	b.isPressed = true
	b.render()

	go func() {
		time.Sleep(100 * time.Millisecond)

		b.app.QueueUpdateDraw(func() {
			b.isPressed = false
			b.render()
		})
	}()
}

//////////////

// Color enum for the circle background
type CircleColor int

const (
	GREEN CircleColor = iota
	YELLOW
	BLUE
	RED
)

// Circle represents the circular component
type Circle struct {
	*tview.Box
	color CircleColor
}

// NewCircle creates a new Circle
func NewCircle() *Circle {
	c := &Circle{
		Box:   tview.NewBox(),
		color: GREEN,
	}
	c.SetBorder(false)
	return c
}

// SetColor sets the circle's color
func (c *Circle) SetColor(color CircleColor) *Circle {
	c.color = color
	return c
}

// Draw draws the circle using a character with the current color
func (c *Circle) Draw(screen tcell.Screen) {
	c.Box.Draw(screen)
	x, y, width, height := c.GetInnerRect()
	if width < 1 || height < 1 {
		return
	}

	var char rune

	style := tcell.StyleDefault
	switch c.color {
	case GREEN:
		char = '🟢'
	case YELLOW:
		char = '🟡'
	case BLUE:
		char = '🔵'
	case RED:
		char = '🔴'
	}

	style = style.Background(tcell.ColorBlack)

	// Draw the circle character at the center of the box
	screen.SetContent(x, y, char, nil, style)
}
