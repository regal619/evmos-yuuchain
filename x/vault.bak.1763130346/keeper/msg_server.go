package keeper

import (
    "context"
    "errors"
    "fmt"

    sdk "github.com/cosmos/cosmos-sdk/types"
    sdkmath "cosmossdk.io/math"

    "github.com/evmos/evmos/v20/x/vault/types"
)

// we hardcode what we currently use on-chain
const (
    // User deposits stablecoin in micro-units (uusdt = 1e-6 USDT)
    depositDenom = "uusdt"

    // Vault mints 18-decimal uyuu (1 YUU = 10^18 uyuu)
    mintDenom = "uyuu"

    scale1e18 = int64(1_000_000_000_000_000_000)
)

type msgServer struct {
    *Keeper
    types.UnimplementedMsgServer
}

func NewMsgServerImpl(k *Keeper) types.MsgServer {
    return &msgServer{Keeper: k}
}

// Deposit takes uusdt from the user and mints Yuu using the bonding-curve price.
func (m msgServer) Deposit(goCtx context.Context, msg *types.MsgDeposit) (*types.MsgDepositResponse, error) {
    ctx := sdk.UnwrapSDKContext(goCtx)

    // 1) who is depositing
    addrBz, err := m.addressCodec.StringToBytes(msg.Creator)
    if err != nil {
        return nil, err
    }
    depositor := sdk.AccAddress(addrBz)

    // 2) take user's uusdt into module
    amt := sdkmath.NewIntFromUint64(msg.Units)
    payCoins := sdk.NewCoins(sdk.NewCoin(depositDenom, amt))
    if err := m.bankKeeper.SendCoinsFromAccountToModule(ctx, depositor, types.ModuleName, payCoins); err != nil {
        return nil, err
    }

    // 3) ask keeper how much to mint (already 1e18-scaled)
    mintAmt, err := m.MintAmountForDeposit(ctx, msg.Units)
    if err != nil {
        return nil, err
    }
    if !mintAmt.IsPositive() {
        return nil, errors.New("deposit too small for current Yuu price")
    }

    // 4) mint to module
    minted := sdk.NewCoins(sdk.NewCoin(mintDenom, mintAmt))
    if err := m.bankKeeper.MintCoins(ctx, types.ModuleName, minted); err != nil {
        return nil, err
    }

    // 5) send to user
    if err := m.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, depositor, minted); err != nil {
        return nil, err
    }

    // 6) update vault accounting in uusdt units
    if err := m.AddDeposit(ctx, depositor, msg.Units); err != nil {
        return nil, err
    }

    // 7) make a human-readable Yuu = uyuu / 1e18 using legacy dec
    human := sdkmath.
        LegacyNewDecFromInt(mintAmt).
        Quo(sdkmath.LegacyNewDec(scale1e18)).
        String()

    // 8) event
    ctx.EventManager().EmitEvent(
        sdk.NewEvent(
            "vault_deposit",
            sdk.NewAttribute("depositor", msg.Creator),
            sdk.NewAttribute("deposit_uusdt", fmt.Sprintf("%d", msg.Units)),
            sdk.NewAttribute("minted_uyuu", mintAmt.String()),
            sdk.NewAttribute("minted_yuu_human", human),
        ),
    )

    // 9) log (handy while testing)
    fmt.Println(
        "[VAULT] Deposit()",
        "depositor", msg.Creator,
        "deposit_uusdt", msg.Units,
        "minted_uyuu", mintAmt.String(),
        "minted_yuu_human", human,
    )

    return &types.MsgDepositResponse{}, nil
}
