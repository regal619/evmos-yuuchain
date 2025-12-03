package types

import (
    "context"

    sdk "github.com/cosmos/cosmos-sdk/types"
)

// BankKeeper defines the subset of the bank keeper methods
// that the vault module actually needs, using context.Context
// to match Evmos' bank keeper.
type BankKeeper interface {
    SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
    SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
    MintCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
}
