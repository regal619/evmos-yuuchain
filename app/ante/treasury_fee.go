package ante

import (
    "context"

    sdk "github.com/cosmos/cosmos-sdk/types"
    authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

// Minimal interfaces so we don't depend on concrete keeper types.

type TreasuryAccountKeeper interface {
    GetModuleAddress(name string) sdk.AccAddress
}

// NOTE: Match the signature used by evmos' BankKeeper:
// SendCoinsFromModuleToAccount(context.Context, string, sdk.AccAddress, sdk.Coins) error
type TreasuryBankKeeper interface {
    SendCoinsFromModuleToAccount(
        ctx context.Context,
        senderModule string,
        recipientAddr sdk.AccAddress,
        amt sdk.Coins,
    ) error
}

// hard-coded treasury address (the one you created)
const TreasuryBech32 = "evmos1ee3psgahr2yxzx9tg6ruxur7n9yzx4q0at4vpk"

// Decode the Bech32 address at use time (after prefixes are configured).
func getTreasuryAddr() sdk.AccAddress {
    addr, err := sdk.AccAddressFromBech32(TreasuryBech32)
    if err != nil {
        panic("invalid treasury bech32 address: " + err.Error())
    }
    return addr
}

// treasuryFeeDecorator moves ALL tx fees from the fee collector
// into the treasury account AFTER normal ante processing runs.
type treasuryFeeDecorator struct {
    ak TreasuryAccountKeeper
    bk TreasuryBankKeeper
}

func NewTreasuryFeeDecorator(ak TreasuryAccountKeeper, bk TreasuryBankKeeper) treasuryFeeDecorator {
    return treasuryFeeDecorator{
        ak: ak,
        bk: bk,
    }
}

func (d treasuryFeeDecorator) AnteHandle(
    ctx sdk.Context,
    tx sdk.Tx,
    simulate bool,
    next sdk.AnteHandler,
) (sdk.Context, error) {
    // 1) Run the rest of the ante chain first (this will deduct the fees).
    newCtx, err := next(ctx, tx, simulate)
    if err != nil {
        return newCtx, err
    }

    // 2) Only care about txs that actually pay fees.
    feeTx, ok := tx.(sdk.FeeTx)
    if !ok {
        return newCtx, nil
    }

    fees := feeTx.GetFee() // sdk.Coins
    if fees.IsZero() {
        return newCtx, nil
    }

    // 3) Move exactly those fee coins from fee collector -> treasury.
    if err := d.bk.SendCoinsFromModuleToAccount(
        sdk.WrapSDKContext(newCtx),             // <- convert sdk.Context -> context.Context
        authtypes.FeeCollectorName,   // "fee_collector"
        getTreasuryAddr(),
        fees,
    ); err != nil {
        return newCtx, err
    }

    return newCtx, nil
}
