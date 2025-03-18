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
	scrollOnce   sync.Once
	setupColumns sync.Once

	sortedColumn *Column
}

func NewTable(title string, columns []Column) *Table {
	t := &Table{
		Table:   tview.NewTable(),
		title:   title,
		columns: columns,
	}

	t.SetFixed(1, 1).
		SetSelectable(true, false).
		SetBorder(true).
		SetBorderPadding(0, 1, 1, 1)

	t.SetSelectedStyle(tcell.Style{}.
		Background(tcell.ColorPurple).
		Foreground(tcell.ColorWhite),
	)

	t.UpdateTitle(0)

	t.DrawHeaders()

	return t
}

func (t *Table) UpdateTitle(count int) {
	t.SetTitle(fmt.Sprintf(" [::b][%s]%s [[%s]%d[%s]] ", tcell.ColorOrange, strings.ToUpper(t.title), tcell.ColorWhiteSmoke, count, tcell.ColorOrange))
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

	t.UpdateTitle(len(rows))
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
