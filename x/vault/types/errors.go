package types

import errorsmod "cosmossdk.io/errors"

var (
	// Used by queries & other generic checks
	ErrInvalidRequest = errorsmod.Register(ModuleName, 1100, "invalid request")

	// Optional: keep if you’ll gate msgs by gov authority
	ErrInvalidSigner = errorsmod.Register(ModuleName, 1101, "expected gov account as only signer for proposal message")
)
