package vault

import (
    "encoding/json"

    "github.com/cosmos/cosmos-sdk/client"
    "github.com/cosmos/cosmos-sdk/codec"
    sdk "github.com/cosmos/cosmos-sdk/types"

    vaultkeeper "github.com/evmos/evmos/v20/x/vault/keeper"
    vaulttypes  "github.com/evmos/evmos/v20/x/vault/types"
)

// AppModule here is a thin wrapper that allows the vault module
// to expose the classic Genesis-related methods. It delegates the
// real work to the keeper and the Genesis helpers in genesis.go.
type AppModule struct {
    cdc    codec.Codec
    keeper vaultkeeper.Keeper
}

// DefaultGenesis → used by 'init' to write genesis.json
func (am AppModule) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
    // DefaultGenesis is defined in x/vault/genesis.go and returns *vaulttypes.GenesisState
    return cdc.MustMarshalJSON(DefaultGenesis())
}

// ValidateGenesis → check params
func (am AppModule) ValidateGenesis(
    cdc codec.JSONCodec,
    _ client.TxEncodingConfig,
    bz json.RawMessage,
) error {
    var gs vaulttypes.GenesisState
    if err := cdc.UnmarshalJSON(bz, &gs); err != nil {
        return err
    }
    return gs.Params.Validate()
}

// InitGenesis → called at block 0
func (am AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, bz json.RawMessage) {
    var gs vaulttypes.GenesisState
    cdc.MustUnmarshalJSON(bz, &gs)
    InitGenesis(ctx, am.keeper, &gs)
}

// ExportGenesis → used by 'export'
func (am AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) json.RawMessage {
    gs := ExportGenesis(ctx, am.keeper)
    return cdc.MustMarshalJSON(gs)
}
