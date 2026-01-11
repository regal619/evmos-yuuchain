package keeper

import (
        "context"

        sdk "github.com/cosmos/cosmos-sdk/types"
        vaulttypes "github.com/evmos/evmos/v20/x/vault/types"
)

// InitGenesis initializes the vault module state from genesis.
func (k Keeper) InitGenesis(ctx context.Context, genState *vaulttypes.GenesisState) {
        sdkCtx := sdk.UnwrapSDKContext(ctx)

        // 1) Params
        params := genState.Params
        // if zero-value, fall back to defaults
        if len(params.AllowedSourceNetworks) == 0 {
                params = vaulttypes.DefaultParams()
        }
        k.SetParams(sdkCtx, params)

        // NOTE:
        // We are NOT restoring any TotalDeposits here because the
        // Evmos-generated GenesisState does not have such a field.
        // The vault will track deposits as they happen via MsgDeposit.
}

// ExportGenesis exports the current vault module state into a GenesisState.
func (k Keeper) ExportGenesis(ctx context.Context) *vaulttypes.GenesisState {
        sdkCtx := sdk.UnwrapSDKContext(ctx)

        params := k.GetParams(sdkCtx)

        return &vaulttypes.GenesisState{
                Params: params,
        }
}
