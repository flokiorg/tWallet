// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package wallet

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/flokiorg/flnd/lnrpc/walletrpc"
	"github.com/flokiorg/go-flokicoin/chainutil"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/flnd"
	"github.com/flokiorg/twallet/shared"
)

const (
	maxAddressDisplayLen = 42
	shortToastAddressLen = 14
)

type addressRow struct {
	TypeLabel string
	Address   string
	Balance   chainutil.Amount
	TxCount   int
}

func (w *Wallet) showUsedAddresses() {
	if w.load == nil || w.load.Wallet == nil {
		return
	}

	w.load.Notif.CancelToast()

	netColor := shared.NetworkColor(*w.load.AppConfig.Network)

	columns := []components.Column{
		{Name: "Type", Align: tview.AlignLeft},
		{Name: "Address", Align: tview.AlignLeft},
		{Name: "Balance", Align: tview.AlignRight},
		{Name: "Tx Count", Align: tview.AlignRight},
	}

	table := components.NewTable("Used Addresses", columns, netColor, 0)
	table.SetBorder(true)
	table.SetBorderColor(tcell.ColorOrange)
	table.SetTitle("")
	table.SetBorderPadding(0, 0, 2, 2)
	table.ShowPlaceholder("Loading addresses...")

	statusView := tview.NewTextView()
	statusView.SetDynamicColors(true)
	statusView.SetTextAlign(tview.AlignLeft)
	statusView.SetBorderPadding(0, 0, 1, 1)
	statusView.SetText("\n[gray::]Total 0 · Showing 0")

	searchField := tview.NewInputField()
	searchField.SetLabel("Search: ")
	searchField.SetFieldWidth(0)
	searchField.SetPlaceholder("address prefix or substring")
	searchField.SetPlaceholderTextColor(tcell.ColorWhite)
	searchField.SetBorder(false)
	searchField.SetBorderPadding(1, 1, 1, 1)

	searchRow := tview.NewFlex().SetDirection(tview.FlexColumn)
	searchRow.SetBackgroundColor(tcell.ColorOrange)
	searchRow.AddItem(tview.NewBox(), 1, 0, false).
		AddItem(searchField, 0, 4, true).
		AddItem(statusView, 0, 2, false).
		AddItem(tview.NewBox(), 1, 0, false)

	container := tview.NewFlex().SetDirection(tview.FlexRow)
	container.SetTitle("Addresses").
		SetTitleColor(tcell.ColorGray).
		SetBorder(true).
		SetBackgroundColor(tcell.ColorOrange)

	container.AddItem(searchRow, 3, 0, true).
		AddItem(table, 0, 1, true)

	allRows := make([]addressRow, 0)
	visibleRows := make([]addressRow, 0)
	totalActive := 0

	countActive := func(rows []addressRow) int {
		total := 0
		for _, row := range rows {
			if row.TxCount > 0 {
				total++
			}
		}
		return total
	}

	updateTotal := func(total, filtered int) {
		statusView.SetText(fmt.Sprintf("\n[gray::]Total %d · Showing %d", total, filtered))
	}

	renderRows := func(rows []addressRow, emptyMsg string) {
		if len(rows) == 0 {
			visibleRows = visibleRows[:0]
			table.ScrollToBeginning()
			table.ShowPlaceholder(emptyMsg)
			updateTotal(totalActive, 0)
			return
		}

		visibleRows = visibleRows[:0]
		data := make([][]string, 0, len(rows))
		shown := 0
		for _, entry := range rows {
			if entry.TxCount == 0 {
				continue
			}
			shown++
			visibleRows = append(visibleRows, entry)

			balance := shared.FormatAmountView(entry.Balance, 6)
			balanceCell := fmt.Sprintf("[gray::]%s", balance)
			if entry.Balance > 0 {
				balanceCell = fmt.Sprintf("[green:-:-]%s", balance)
			}
			typeCell := entry.TypeLabel
			switch entry.TypeLabel {
			case "Change":
				typeCell = "[yellow:-:-]Change"
			case "External":
				typeCell = "[green:-:-]External"
			default:
				typeCell = fmt.Sprintf("[gray::]%s", entry.TypeLabel)
			}
			txCell := fmt.Sprintf("[gray::]%d", entry.TxCount)
			if entry.TxCount > 0 {
				txCell = fmt.Sprintf("[%s:-:-]%d", tcell.ColorLightSkyBlue, entry.TxCount)
			}
			displayAddr := shortenAddressForDisplay(entry.Address)
			data = append(data, []string{
				typeCell,
				displayAddr,
				balanceCell,
				txCell,
			})
		}

		if shown == 0 {
			visibleRows = visibleRows[:0]
			table.ScrollToBeginning()
			table.ShowPlaceholder(emptyMsg)
			updateTotal(totalActive, 0)
			return
		}

		table.Update(data)
		table.Select(1, 0)
		table.ScrollToBeginning()
		updateTotal(totalActive, shown)
	}

	applyFilter := func(query string) {
		if len(allRows) == 0 {
			renderRows(allRows, "No addresses yet")
			return
		}

		q := strings.TrimSpace(strings.ToLower(query))
		if q == "" {
			renderRows(allRows, "No addresses yet")
			return
		}

		filtered := make([]addressRow, 0)
		for _, row := range allRows {
			addr := strings.ToLower(row.Address)
			typeLabel := strings.ToLower(row.TypeLabel)
			if strings.Contains(addr, q) || strings.Contains(typeLabel, q) {
				filtered = append(filtered, row)
			}
		}

		emptyMsg := "No addresses found"
		renderRows(filtered, emptyMsg)
	}

	clearSearch := func() {
		if strings.TrimSpace(searchField.GetText()) != "" {
			searchField.SetText("")
		}
		applyFilter("")
		w.load.Application.SetFocus(searchField)
	}

	copyAddress := func(row int) {
		if row <= 0 || row-1 >= len(visibleRows) {
			return
		}
		entry := visibleRows[row-1]
		if err := shared.ClipboardCopy(entry.Address); err != nil {
			w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*10)
			return
		}
		w.load.Notif.ShowToastWithTimeout(
			fmt.Sprintf("📋 Copied %s (%s)", shortAddress(entry.Address), entry.TypeLabel),
			time.Second*10,
		)
	}

	table.SetSelectedFunc(func(row int, column int) {
		copyAddress(row)
	})

	table.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			clearSearch()
		}
	})

	searchField.SetChangedFunc(func(text string) {
		applyFilter(text)
	})

	searchField.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			w.load.Application.SetFocus(table)
		case tcell.KeyEscape:
			clearSearch()
		}
	})

	container.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyEsc:
			if strings.TrimSpace(searchField.GetText()) == "" {
				w.closeModal()
			} else {
				clearSearch()
			}
			return nil
		case event.Key() == tcell.KeyCtrlC:
			w.closeModal()
			return nil
		}
		return event
	})

	w.nav.ShowModal(components.NewModal(container, 96, 30, w.closeModal))
	w.load.Application.SetFocus(searchField)

	go func() {
		accounts, err := w.load.Wallet.ListAddresses()
		txCounts, txErr := w.addressTransactionCounts()

		if txCounts == nil {
			txCounts = map[string]int{}
		}

		w.load.Application.QueueUpdateDraw(func() {
			if err != nil {
				table.ShowPlaceholder("Unable to load addresses")
				w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[red:-:-]Error:[-:-:-] %s", err.Error()), time.Second*20)
				updateTotal(0, 0)
				return
			}
			if txErr != nil {
				w.load.Notif.ShowToastWithTimeout(fmt.Sprintf("[yellow:-:-]Warning:[-:-:-] transactions unavailable: %s", txErr.Error()), time.Second*15)
			}

			allRows = buildAddressRows(accounts, txCounts)
			totalActive = countActive(allRows)
			applyFilter(strings.TrimSpace(searchField.GetText()))
		})
	}()
}

func buildAddressRows(accounts []*walletrpc.AccountWithAddresses, txCounts map[string]int) []addressRow {
	rows := make([]addressRow, 0)
	for _, acct := range accounts {
		if acct == nil {
			continue
		}
		for _, addr := range acct.GetAddresses() {
			if addr == nil {
				continue
			}
			address := strings.TrimSpace(addr.GetAddress())
			if address == "" {
				continue
			}
			balance := chainutil.Amount(addr.GetBalance())
			typeLabel := "External"
			if addr.GetIsInternal() {
				typeLabel = "Change"
			}

			rows = append(rows, addressRow{
				TypeLabel: typeLabel,
				Address:   address,
				Balance:   balance,
				TxCount:   txCounts[address],
			})
		}
	}

	typePriority := func(label string) int {
		switch strings.ToLower(label) {
		case "external":
			return 0
		case "change":
			return 1
		default:
			return 2
		}
	}

	category := func(row addressRow) int {
		switch {
		case row.Balance > 0:
			return 0
		case row.TxCount > 0:
			return 1
		case strings.ToLower(row.TypeLabel) == "external":
			return 2
		default:
			return 3
		}
	}

	sort.SliceStable(rows, func(i, j int) bool {
		left, right := rows[i], rows[j]

		leftCat, rightCat := category(left), category(right)
		if leftCat != rightCat {
			return leftCat < rightCat
		}

		switch leftCat {
		case 0:
			if left.Balance != right.Balance {
				return left.Balance > right.Balance
			}
			if left.TxCount != right.TxCount {
				return left.TxCount > right.TxCount
			}
		case 1:
			if left.TxCount != right.TxCount {
				return left.TxCount > right.TxCount
			}
		}

		leftType, rightType := typePriority(left.TypeLabel), typePriority(right.TypeLabel)
		if leftType != rightType {
			return leftType < rightType
		}

		if left.Balance != right.Balance {
			return left.Balance > right.Balance
		}
		if left.TxCount != right.TxCount {
			return left.TxCount > right.TxCount
		}

		return left.Address < right.Address
	})

	return rows
}

func shortAddress(addr string) string {
	if len(addr) <= shortToastAddressLen {
		return addr
	}
	return fmt.Sprintf("%s...%s", addr[:6], addr[len(addr)-6:])
}

func (w *Wallet) addressTransactionCounts() (map[string]int, error) {
	txs, err := w.load.Wallet.FetchTransactionsWithOptions(flnd.FetchTransactionsOptions{
		IgnoreLimit: true,
	})
	if err != nil {
		return map[string]int{}, err
	}

	counts := make(map[string]int)
	for _, tx := range txs {
		if tx == nil {
			continue
		}
		seen := make(map[string]struct{})
		for _, detail := range tx.GetOutputDetails() {
			if detail == nil {
				continue
			}
			addr := strings.TrimSpace(detail.Address)
			if addr == "" {
				continue
			}
			if _, ok := seen[addr]; ok {
				continue
			}
			counts[addr]++
			seen[addr] = struct{}{}
		}
	}

	return counts, nil
}

func shortenAddressForDisplay(address string) string {
	if len(address) <= maxAddressDisplayLen {
		return address
	}
	if maxAddressDisplayLen <= 3 {
		return address[:maxAddressDisplayLen]
	}

	keep := maxAddressDisplayLen - 3
	front := keep / 2
	back := keep - front

	return fmt.Sprintf("%s...%s", address[:front], address[len(address)-back:])
}
