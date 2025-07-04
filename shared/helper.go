// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package shared

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/flokiorg/go-flokicoin/chaincfg"
	"github.com/flokiorg/go-flokicoin/chainutil"
	"github.com/flokiorg/go-flokicoin/wire"
	"github.com/gdamore/tcell/v2"
	"github.com/skip2/go-qrcode"
)

const (
	flcSign = "𝔽"
)

func FormatAmountView(value chainutil.Amount, precision int) string {
	// Check if the value is negative
	isNegative := value < 0
	if isNegative {
		value = -value // Convert to positive for formatting
	}

	// Format the number with the specified precision
	formatted := fmt.Sprintf("%.*f", precision, value.ToFLC())

	// Split into integer and decimal parts
	parts := strings.Split(formatted, ".")
	intPart := parts[0]

	// Use a strings.Builder for efficient concatenation
	var intWithCommas strings.Builder
	length := len(intPart)

	// Add commas to the integer part
	for i, v := range intPart {
		if i > 0 && (length-i)%3 == 0 {
			intWithCommas.WriteByte(',')
		}
		intWithCommas.WriteByte(byte(v))
	}

	// Process decimal part
	var finalAmount string
	if len(parts) > 1 {
		decimalPart := strings.TrimRight(parts[1], "0") // Remove trailing zeros
		if decimalPart == "" {
			finalAmount = intWithCommas.String()
		} else {
			finalAmount = intWithCommas.String() + "." + decimalPart
		}
	} else {
		finalAmount = intWithCommas.String()
	}

	// Add currency sign and handle negative numbers
	if isNegative {
		return fmt.Sprintf("-%s %s", finalAmount, flcSign) // Negative formatting
	}
	return fmt.Sprintf("%s %s", finalAmount, flcSign)
}

func ClipboardCopy(text string) error {
	if err := clipboard.WriteAll(text); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return fmt.Errorf("right-click to copy (no display)")
		}
		return fmt.Errorf("clipboard copy failed: %w", err)
	}
	return nil
}

func NetworkColor(network chaincfg.Params) tcell.Color {
	var logoColor tcell.Color

	switch network.Net {
	case wire.MainNet:
		logoColor = tcell.ColorOrange
	case wire.TestNet3:
		logoColor = tcell.ColorRed
	default:
		logoColor = tcell.ColorYellowGreen
	}

	return logoColor
}

func GenerateQRText(txt string) (string, error) {
	qr, err := qrcode.New(txt, qrcode.High)
	if err != nil {
		return "", err
	}
	qr.DisableBorder = true
	return qr.ToSmallString(true), err
}
