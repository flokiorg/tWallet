// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package components

import (
	"fmt"
	"strings"
	"sync"

	"github.com/rivo/tview"

	"github.com/gdamore/tcell/v2"
)

type SortOrder int

const (
	Ascending SortOrder = iota
	Descending
)

type Column struct {
	Name     string
	Align    int
	IsSorted bool
	SortDir  SortOrder
}

type Table struct {
	*tview.Table
	title string

	columns []Column
	// rows         *FLowMetricsSlice
	scrollOnce sync.Once
	netColor   tcell.Color
	maxRows    int
}

func NewTable(title string, columns []Column, netColor tcell.Color, maxRows int) *Table {
	t := &Table{
		Table:    tview.NewTable(),
		title:    title,
		columns:  columns,
		netColor: netColor,
		maxRows:  maxRows,
	}

	t.SetFixed(1, 1).
		SetSelectable(true, false).
		SetBorder(true).
		SetBorderPadding(0, 1, 1, 1)

	t.SetSelectedStyle(tcell.Style{}.
		Background(tcell.ColorPurple).
		Foreground(tcell.ColorWhite),
	)

	t.UpdateTitle(0, false)

	t.DrawHeaders()

	return t
}

func (t *Table) UpdateTitle(count int, hasMore bool) {
	strCount := fmt.Sprintf("%d", count)
	if hasMore {
		strCount = fmt.Sprintf("%d+", count)
	}

	t.SetTitle(fmt.Sprintf(" [::b][%s]%s [[%s]%s[%s]] ", t.netColor, strings.ToUpper(t.title), tcell.ColorWhiteSmoke, strCount, t.netColor))
}

func (t *Table) DrawHeaders() {

	for cid, column := range t.columns {
		header := fmt.Sprintf("[%s:-:b]%s", tcell.ColorGray, strings.ToUpper(column.Name))
		if column.IsSorted {
			switch column.SortDir {
			case Ascending:
				header += fmt.Sprintf("[%s:-:-]↑", tcell.ColorPurple)

			case Descending:
				header += fmt.Sprintf("[%s:-:-]↓", tcell.ColorPurple)
			}
		}
		t.SetCell(0, cid,
			tview.NewTableCell(header).
				SetExpansion(1).
				SetTextColor(tcell.ColorBlack).
				SetAlign(column.Align).
				SetSelectable(false))
	}

}

func (t *Table) Update(rows [][]string) {
	if rows == nil {
		return
	}

	t.Clear()

	t.UpdateTitle(len(rows), false)
	t.DrawHeaders()

	for rid, row := range rows {
		for cid, column := range row {
			t.SetCell(rid+1, cid, tview.NewTableCell(column).
				SetExpansion(1).
				SetAlign(t.columns[cid].Align))
		}
	}

	t.scrollOnce.Do(func() {
		t.ScrollToBeginning()
	})
}

func (t *Table) ShowPlaceholder(message string) {
	if len(t.columns) == 0 {
		return
	}

	t.Clear()

	t.UpdateTitle(0, false)
	t.DrawHeaders()

	placeholder := message

	_, _, _, innerHeight := t.GetInnerRect()
	if innerHeight <= 0 {
		_, _, _, outerHeight := t.GetRect()
		if outerHeight <= 0 {
			outerHeight = 6
		}
		innerHeight = outerHeight - 2
		if innerHeight <= 0 {
			innerHeight = outerHeight
		}
	}
	placeholderText := fmt.Sprintf("[gray::]%s", placeholder)

	contentRows := innerHeight - 1
	if contentRows < 1 {
		contentRows = 1
	}
	placeholderRow := 1 + contentRows/2
	if placeholderRow < 1 {
		placeholderRow = 1
	}
	if t.maxRows > 0 && placeholderRow > t.maxRows {
		placeholderRow = t.maxRows
	}

	blanks := func() *tview.TableCell {
		return tview.NewTableCell("").
			SetSelectable(false).
			SetAlign(tview.AlignLeft).
			SetExpansion(1)
	}

	for row := 1; row < placeholderRow; row++ {
		for cid := range t.columns {
			t.SetCell(row, cid, blanks())
		}
	}

	centerCol := len(t.columns) / 2
	for cid := range t.columns {
		cell := blanks()
		if cid == centerCol {
			cell = tview.NewTableCell(placeholderText).
				SetAlign(tview.AlignCenter).
				SetExpansion(1).
				SetSelectable(false)
		}
		t.SetCell(placeholderRow, cid, cell)
	}

	// Allow future updates to reposition the view at the top.
	t.scrollOnce = sync.Once{}
}
