package shared

type Page int

const (
	WALLET Page = iota
	LOCK
	ONBOARD
	CHANGE
)

const (
	MinPasswordLength   = 8
	MinWalletNameLength = 3
)
