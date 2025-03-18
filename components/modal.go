// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package components

import (
	"github.com/rivo/tview"

	"github.com/gdamore/tcell/v2"
)

type Dialog struct {
	*tview.Modal
}

func NewModal(p tview.Primitive, width, height int, escFunc func()) tview.Primitive {

	view := tview.NewFlex().SetDirection(tview.FlexRow)
	view.AddItem(nil, 0, 1, false).
		AddItem(p, height, 1, true).
		AddItem(nil, 0, 1, false)

	modal := tview.NewFlex()
	modal.SetBackgroundColor(tcell.ColorOrange)
	modal.AddItem(nil, 0, 1, false).
		AddItem(view, width, 1, true).
		AddItem(nil, 0, 1, false)

	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if escFunc != nil && event.Key() == tcell.KeyESC {
			escFunc()
		}

		return event
	})

	return modal
}

func NewDialog(title, text string, escFunc func(), buttons []string, funcs ...func()) *Dialog {

	modal := tview.NewModal()
	modal.SetTitle(title)
	modal.SetText(text)
	modal.SetBackgroundColor(tcell.ColorDefault)
	modal.Box.SetBackgroundColor(tcell.ColorDefault)
	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if escFunc != nil && event.Key() == tcell.KeyESC {
			escFunc()
		}

		return event
	})

	if buttons != nil {
		modal.AddButtons(buttons).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				if buttonIndex >= 0 && buttonIndex < len(funcs) {
					funcs[buttonIndex]()
				} else if len(funcs) >= 0 { // Esc key
					funcs[len(funcs)-1]()
				}
			})
	}

	return &Dialog{
		Modal: modal,
	}

}

func ErrorModal(text string, closeFunc func()) tview.Primitive {
	m := NewDialog("Error", text, closeFunc, []string{"OK"}, closeFunc)
	m.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if closeFunc == nil || event.Key() != tcell.KeyESC {
			return event
		}
		closeFunc()
		return event
	})

	return m
}

func Toast(text string) tview.Primitive {
	t := tview.NewTextView()
	t.SetText(text).
		SetDynamicColors(true).
		SetTextColor(tcell.ColorWhite).
		SetTextAlign(tview.AlignCenter)

	return NewModal(t, 50, 3, nil)
}

// func (a *App) showDialog(title, text string, textColor tcell.Color, buttons []string, funcs ...func()) {
// 	a.dialog = NewDialog(title, text, buttons, funcs...)
// 	a.dialog.SetTextColor(textColor).SetText(text).SetTitle(title)

// 	a.pages.RemovePage("dialog").AddPage("dialog", a.dialog, true, true)
// }

// func (a *App) Debug(text string) {
// 	a.showDialog("Debug", text, tcell.ColorDefault, []string{"OK"},
// 		func() {
// 			a.pages.SwitchToPage("main")
// 			a.Focus(a.lastFocusedIndex)
// 		},
// 	)
// }

// func (a *App) Alert(text string) {
// 	a.showDialog("Info", text, tcell.ColorDefault, []string{"OK"},
// 		func() {
// 			a.pages.SwitchToPage("main")
// 			a.Focus(a.lastFocusedIndex)
// 		},
// 	)
// }

// func (a *App) ConfirmDelete(text string, confirmFunc func()) {
// 	a.showDialog("Delete?", text, tcell.ColorDefault, []string{"OK", "Cancel"},
// 		func() {
// 			confirmFunc()
// 			a.pages.SwitchToPage("main")
// 			a.Focus(a.lastFocusedIndex)
// 		},
// 		func() {
// 			a.pages.SwitchToPage("main")
// 			a.Focus(a.lastFocusedIndex)
// 		},
// 	)
// }
