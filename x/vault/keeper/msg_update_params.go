package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
        vaulttypes "github.com/evmos/evmos/v20/x/vault/types"
)

func (m msgServer) UpdateParams(ctx context.Context, msg *vaulttypes.MsgUpdateParams) (*vaulttypes.MsgUpdateParamsResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// authority check
	if msg.Authority != m.Keeper.GetAuthority() {
		return nil, errorsmod.Wrap(vaulttypes.ErrInvalidSigner, "invalid authority")
	}

// msg.Params is a value; validate and set directly
        if err := msg.Params.Validate(); err != nil {
                return nil, err
        }
        m.Keeper.SetParams(sdkCtx, *msg.Params)

        return &vaulttypes.MsgUpdateParamsResponse{}, nil
}
