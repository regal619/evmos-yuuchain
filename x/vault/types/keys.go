package types

import "cosmossdk.io/collections"

const (
	// Module name and store key
	ModuleName = "vault"
	StoreKey   = ModuleName

	// Module account name (used to derive the vault’s module address)
	ModuleAccountName = ModuleName
)

var (
	// Global totals
	KeyTotalDeposits = []byte{0x00} // total USD-equivalent deposits
	KeyTotalMinted   = []byte{0x01} // total YUU minted

	// Per-address deposit prefix
	KeyPrefixDeposits = []byte{0x02}

	// Params key (future use)
	ParamsKey = collections.NewPrefix(0)
)
