package vault

import (
        sdk "github.com/cosmos/cosmos-sdk/types"

        vaultkeeper "github.com/evmos/evmos/v20/x/vault/keeper"
        vaulttypes "github.com/evmos/evmos/v20/x/vault/types"
)

// GenesisState is an alias to the protobuf-generated type in vault/types.
type GenesisState = vaulttypes.GenesisState

// DefaultGenesis returns the default vault genesis state.
func DefaultGenesis() *GenesisState {
        return &GenesisState{
                Params: vaulttypes.DefaultParams(),
        }
}

// InitGenesis initializes the vault module state from genesis.
func InitGenesis(ctx sdk.Context, k vaultkeeper.Keeper, genState *GenesisState) {
        // keeper.InitGenesis expects context.Context, so wrap sdk.Context.
        k.InitGenesis(sdk.WrapSDKContext(ctx), genState)
}

// ExportGenesis exports the current vault module state into a GenesisState.
func ExportGenesis(ctx sdk.Context, k vaultkeeper.Keeper) *GenesisState {
        return k.ExportGenesis(sdk.WrapSDKContext(ctx))
}
