package module

import (
    "context"
    "encoding/json"
    "fmt"

    abci "github.com/cometbft/cometbft/abci/types"
    "github.com/cosmos/cosmos-sdk/client"
    "github.com/cosmos/cosmos-sdk/codec"
    codectypes "github.com/cosmos/cosmos-sdk/codec/types"
    sdk "github.com/cosmos/cosmos-sdk/types"
    "github.com/cosmos/cosmos-sdk/types/module"

    "github.com/spf13/cobra"
    "github.com/evmos/evmos/v20/x/vault/client/cli"

    appmodule "cosmossdk.io/core/appmodule"

    vaultkeeper "github.com/evmos/evmos/v20/x/vault/keeper"
    vaulttypes "github.com/evmos/evmos/v20/x/vault/types"

    // for gRPC-gateway registration
    gwruntime "github.com/grpc-ecosystem/grpc-gateway/runtime"
)

// Make sure we implement the required interfaces.
var (
    _ module.AppModuleBasic   = AppModuleBasic{}
    _ module.AppModule        = AppModule{}
    _ appmodule.AppModule     = AppModule{}
)

// -----------------------------------------------------------------------------
// AppModuleBasic
// -----------------------------------------------------------------------------

type AppModuleBasic struct{}

// Name returns the vault module name.
func (AppModuleBasic) Name() string { return vaulttypes.ModuleName }

func (AppModuleBasic) RegisterLegacyAminoCodec(_ *codec.LegacyAmino) {}

// RegisterInterfaces registers the vault interfaces (Msg / Query, etc.).
func (AppModuleBasic) RegisterInterfaces(reg codectypes.InterfaceRegistry) {
    vaulttypes.RegisterInterfaces(reg)
}

// DefaultGenesis is used to initialize the vault genesis state.
func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
    return cdc.MustMarshalJSON(vaulttypes.DefaultGenesis())
}

// ValidateGenesis validates the genesis state.
func (AppModuleBasic) ValidateGenesis(
    cdc codec.JSONCodec,
    _ client.TxEncodingConfig,
    bz json.RawMessage,
) error {
    var gs vaulttypes.GenesisState
    if err := cdc.UnmarshalJSON(bz, &gs); err != nil {
        return err
    }
    return gs.Validate()
}

// RegisterGRPCGatewayRoutes wires the gRPC query service to the HTTP gateway.
func (AppModuleBasic) RegisterGRPCGatewayRoutes(
    clientCtx client.Context,
    mux *gwruntime.ServeMux,
) {
    if err := vaulttypes.RegisterQueryHandlerClient(
        context.Background(),
        mux,
        vaulttypes.NewQueryClient(clientCtx),
    ); err != nil {
        panic(err)
    }
}

// We’re not wiring CLI here (tx/query commands) – they can be added later.
// This keeps compilation simple and avoids vaultcli.NewTxCmd errors.
// func (AppModuleBasic) GetTxCmd() *cobra.Command    { return nil }
func (AppModuleBasic) GetQueryCmd() *cobra.Command {
	return cli.GetQueryCmd()
}
// -----------------------------------------------------------------------------
// AppModule
// -----------------------------------------------------------------------------

// AppModule connects the keeper to the module system.
type AppModule struct {
    AppModuleBasic
    keeper *vaultkeeper.Keeper
    cdc    codec.Codec
}

// These marker methods are required for appmodule wiring.
func (AppModule) IsAppModule()        {}
func (AppModule) IsOnePerModuleType() {}

// NewAppModule creates a new AppModule instance.
func NewAppModule(cdc codec.Codec, k *vaultkeeper.Keeper) AppModule {
    return AppModule{
        AppModuleBasic: AppModuleBasic{},
        keeper:         k,
        cdc:            cdc,
    }
}

// RegisterServices wires the Msg and Query servers.
func (am AppModule) RegisterServices(cfg module.Configurator) {
    fmt.Println("[VAULT] RegisterServices: begin")
    vaulttypes.RegisterMsgServer(cfg.MsgServer(), vaultkeeper.NewMsgServerImpl(am.keeper))
    vaulttypes.RegisterQueryServer(cfg.QueryServer(), am.keeper)
    fmt.Println("[VAULT] RegisterServices: query server registered")
}

// Invariants: none for vault right now.
func (AppModule) RegisterInvariants(_ sdk.InvariantRegistry) {}

// InitGenesis initializes state from genesis.json.
func (am AppModule) InitGenesis(
    ctx sdk.Context,
    cdc codec.JSONCodec,
    data json.RawMessage,
) []abci.ValidatorUpdate {
    var gs vaulttypes.GenesisState
    cdc.MustUnmarshalJSON(data, &gs)

    // At minimum: set params on the keeper
    am.keeper.SetParams(ctx, gs.Params)

    return []abci.ValidatorUpdate{}
}

// ExportGenesis exports the current vault state to genesis format.
func (am AppModule) ExportGenesis(
    ctx sdk.Context,
    cdc codec.JSONCodec,
) json.RawMessage {
    params := am.keeper.GetParams(ctx)

    gs := &vaulttypes.GenesisState{
        Params: params,
    }

    return cdc.MustMarshalJSON(gs)
}

// ConsensusVersion is used for migrations (1 for now).
func (AppModule) ConsensusVersion() uint64 { return 1 }
