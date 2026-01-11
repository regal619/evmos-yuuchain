package keeper

import (
        "context"

        sdk "github.com/cosmos/cosmos-sdk/types"
        vaulttypes "github.com/evmos/evmos/v20/x/vault/types"
)

// Make Keeper satisfy the generated gRPC QueryServer.
var _ vaulttypes.QueryServer = (*Keeper)(nil)

// grpc-gen requires this method when you don't embed UnimplementedQueryServer.
func (Keeper) mustEmbedUnimplementedQueryServer() {}

// Keep app compatibility: app/app.go is calling vaultkeeper.NewQueryServer(..)
func NewQueryServer(k *Keeper) vaulttypes.QueryServer { return k }

// Params returns the module parameters.
func (k *Keeper) Params(ctx context.Context, req *vaulttypes.QueryParamsRequest) (*vaulttypes.QueryParamsResponse, error) {
        if req == nil {
                return nil, vaulttypes.ErrInvalidRequest
        }
        sdkCtx := sdk.UnwrapSDKContext(ctx)
        p := k.GetParams(sdkCtx) // value
        return &vaulttypes.QueryParamsResponse{Params: &p}, nil
}

// Deposit returns the recorded deposit amount for an address.
func (k *Keeper) Deposit(ctx context.Context, req *vaulttypes.DepositRequest) (*vaulttypes.DepositResponse, error) {
        if req == nil || req.Address == "" {
                return nil, vaulttypes.ErrInvalidRequest
        }
        sdkCtx := sdk.UnwrapSDKContext(ctx)

        addr, err := sdk.AccAddressFromBech32(req.Address)
        if err != nil {
                return nil, err
        }
        amt := k.GetDeposit(sdkCtx, addr)
        return &vaulttypes.DepositResponse{Amount: amt.String()}, nil
}

// Total returns the total recorded deposits across all accounts.
func (k *Keeper) Total(ctx context.Context, req *vaulttypes.TotalRequest) (*vaulttypes.TotalResponse, error) {
        if req == nil {
                return nil, vaulttypes.ErrInvalidRequest
        }
        sdkCtx := sdk.UnwrapSDKContext(ctx)
        total := k.GetTotal(sdkCtx)
        return &vaulttypes.TotalResponse{Total: total.String()}, nil
}
