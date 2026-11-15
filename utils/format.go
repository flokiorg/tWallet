// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package utils

import (
	"fmt"
	"strconv"

	"github.com/flokiorg/go-flokicoin/chainutil"
)

type FeeOption struct {
	Label  string
	Amount chainutil.Amount
}

func ParseIntWithDefault(value string, defaultValue int) (int, error) {
	if value == "" {
		return defaultValue, nil
	}
	return strconv.Atoi(value)
}

func BuildFeesOptions(slow, medium, fast float64) []FeeOption {
	return []FeeOption{
		{Label: fmt.Sprintf(" Slow: %.0f loki/vB ", slow), Amount: chainutil.Amount(slow)},
		{Label: fmt.Sprintf(" Medium: %.0f loki/vB ", medium), Amount: chainutil.Amount(medium)},
		{Label: fmt.Sprintf(" Fast: %.0f loki/vB ", fast), Amount: chainutil.Amount(fast)},
	}
}
