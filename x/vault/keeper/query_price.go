package keeper

import (
    "context"

    sdk "github.com/cosmos/cosmos-sdk/types"

    vaulttypes "github.com/evmos/evmos/v20/x/vault/types"
)

// Price implements the gRPC query to get current Yuu price.
func (k Keeper) Price(goCtx context.Context, _ *vaulttypes.PriceRequest) (*vaulttypes.PriceResponse, error) {
    // we don’t actually need the sdk.Context right now, but this is the usual pattern
    sdkCtx := sdk.UnwrapSDKContext(goCtx)
    _ = sdkCtx

    dec := k.GetYuuPrice(goCtx) // returns sdkmath.LegacyDec
    return &vaulttypes.PriceResponse{
        Price: dec.String(),
    }, nil
}
