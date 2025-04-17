// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package components

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"

	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/shared"
	. "github.com/flokiorg/twallet/shared"
	"github.com/gdamore/tcell/v2"
)

type Cipher struct {
	*tview.Flex
	load *load.Load

	wordsGrid *tview.Grid
	hexView   *tview.TextView
	copyBtn   *ConfirmButton

	container *tview.Grid
	words     []string
	hex, wif  string
}

// NewCipher creates and initializes a new Cipher instance with the provided words.
// It sets up the UI components and populates them with the cipher data.
func NewCipher(l *load.Load, mnemonic []string, hex string) (*Cipher, int, error) {
	c := &Cipher{
		Flex:      tview.NewFlex(),
		wordsGrid: tview.NewGrid(),
		hexView:   tview.NewTextView(),

		words: mnemonic,
		hex:   hex,
		load:  l,
	}

	// Configure the wordsGrid
	c.wordsGrid.
		SetColumns(0, 0, 0). // Three flexible columns
		SetBorders(false).SetBorderPadding(1, 1, 1, 1)

	// Configure the hex TextView
	c.hexView.
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).SetWrap(true)

	c.copyBtn = NewConfirmButton(l.Application, "copy", false, tcell.ColorBlack, 1, c.copyToClipboard)

	if err := c.Update(mnemonic, hex); err != nil {
		return nil, 0, err
	}

	c.container = tview.NewGrid().SetRows(0, 5, 1).
		SetColumns(0, 0, 0).
		AddItem(c.wordsGrid, 0, 0, 1, 3, 0, 0, false).
		AddItem(c.hexView, 1, 0, 1, 3, 0, 0, false).
		AddItem(c.copyBtn, 2, 2, 1, 1, 0, 0, false)

	c.container.SetBorder(true).SetTitle("Cipher Card")

	c.AddItem(c.container, 0, 1, true)

	return c, calcViewHeight(MnemonicLen(len(mnemonic))), nil
}

// SetWords sets the cipher words and updates the hex
// It validates the word count and formats the displays accordingly.
func (c *Cipher) Update(words []string, hex string) error {
	// Validate word count
	if !IsValidMnemonicLen(MnemonicLen(len(words))) {
		return fmt.Errorf("invalid seed phrase: %d words", len(words))
	}

	c.wordsGrid.Clear()
	c.wordsGrid.SetRows(makeRows(len(words))...)

	// Add words to the wordsGrid in 3 columns
	for i, word := range words {
		row := i / 3
		col := i % 3

		wordText := fmt.Sprintf("[orange:-:-]%2d.[-:-:-] %s", i+1, word)

		tv := tview.NewTextView().
			SetDynamicColors(true).
			SetTextAlign(tview.AlignLeft).
			SetText(wordText)

		c.wordsGrid.AddItem(tv, row, col, 1, 1, 0, 0, false)
	}

	c.hexView.SetText(fmt.Sprintf("[orange:-:-]%s[-:-:-] %s", "Hex: ", hex)).SetBorderPadding(1, 1, 1, 1)

	c.words = words
	c.hex = hex

	return nil
}

func (c *Cipher) copyToClipboard() {
	var sb strings.Builder

	// Gather cipher words
	sb.WriteString("Mnemonic:\n")
	for i, word := range c.words {
		sb.WriteString(fmt.Sprintf("[%d] %s ", i+1, word))
		if (i+1)%3 == 0 {
			sb.WriteString("\n")
		}
	}

	sb.WriteString(fmt.Sprintf("\nInline Mnemonic:\n%s\n", strings.Join(c.words, " ")))

	// Gather hex data
	hexText := c.hexView.GetText(true)
	sb.WriteString("\n" + hexText + "\n")

	// Copy to clipboard
	err := shared.ClipboardCopy(sb.String())
	if err != nil {
		c.load.Nav.ShowModal(ErrorModal(err.Error(), c.load.Nav.CloseModal))
	}

}

// deprecated
func (c *Cipher) SetBackgroundColor(color tcell.Color) {
	c.Flex.SetBackgroundColor(color)
	c.wordsGrid.SetBackgroundColor(color)
	c.hexView.SetBackgroundColor(color)
	c.container.SetBackgroundColor(color)
	c.copyBtn.SetBackgroundColor(color)
}

// makeRows calculates the number of rows needed based on the word count.
// Each row can contain up to 3 words.
func makeRows(wordCount int) []int {
	rows := wordCount / 3
	if wordCount%3 != 0 {
		rows += 1
	}
	rowSizes := make([]int, rows)
	for i := range rowSizes {
		rowSizes[i] = 1 // Each row has a flexible height
	}
	return rowSizes
}

func calcViewHeight(len MnemonicLen) int {
	switch len {

	case W18:
		return 16
	case W24:
		return 19

	// case W12:
	default:
		return 14
	}
}
