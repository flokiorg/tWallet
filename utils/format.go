// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package utils

import "strconv"

func ParseIntWithDefault(value string, defaultValue int) (int, error) {
	if value == "" {
		return defaultValue, nil
	}
	return strconv.Atoi(value)
}
