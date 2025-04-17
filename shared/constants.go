// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package shared

import "fmt"

var (
	WELCOME_MESSAGE = fmt.Sprintf("[orange:-:b]%s[-:-:-] %s", "Flokicoin", "take the meme fun to the 5 level")
	LOGO_TEXT       = `
  ___|_|__  _     ____ 
 |  _|_|__|| |   / ___|
 | |_|_|_  | |  | |   
 |   |_|_| | |__| |___ 
 |__|| |   |_____\____|
     | |
`

	SPLASH_LOGO_TEXT = `
  ___X_X__  _     ____ 
 |  _X_X__|| |   / ___|
 | |_X_X_  | |  | |   
 |   X_X_| | |__| |___ 
 |__|X X   |_____\____|
     X X
`
)

const (
	MinPasswordLength   = 5
	MinWalletNameLength = 3
)
