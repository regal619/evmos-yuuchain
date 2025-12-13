package keeper

import (
    "context"
    "errors"
    "fmt"
    "strings"

    sdk "github.com/cosmos/cosmos-sdk/types"
    sdkmath "cosmossdk.io/math"
    vaulttypes "github.com/evmos/evmos/v20/x/vault/types"
)

// we hardcode what we currently use on-chain
const (
    depositDenom = "uusdt" // user deposits this
    mintDenom    = "uyuu"  // vault mints this (18 decimals)
    scale1e18    = int64(1_000_000_000_000_000_000)
)

type msgServer struct {
    *Keeper
    vaulttypes.UnimplementedMsgServer
}

func NewMsgServerImpl(k *Keeper) vaulttypes.MsgServer {
    return &msgServer{Keeper: k}
}

// Deposit takes uusdt from the user and mints Yuu using the bonding-curve price.
func (m msgServer) Deposit(goCtx context.Context, msg *vaulttypes.MsgDeposit) (*vaulttypes.MsgDepositResponse, error) {
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
    if err := m.bankKeeper.SendCoinsFromAccountToModule(goCtx, depositor, vaulttypes.ModuleName, payCoins); err != nil {
        return nil, err
    }

    // 3) ask keeper how much to mint (already 1e18-scaled)
    mintAmt, err := m.MintAmountForDeposit(goCtx, msg.Units)
    if err != nil {
        return nil, err
    }
    if !mintAmt.IsPositive() {
        return nil, errors.New("deposit too small for current Yuu price")
    }

    // 4) mint to module
    minted := sdk.NewCoins(sdk.NewCoin(mintDenom, mintAmt))
    if err := m.bankKeeper.MintCoins(goCtx, vaulttypes.ModuleName, minted); err != nil {
        return nil, err
    }

    // 5) send to user
    if err := m.bankKeeper.SendCoinsFromModuleToAccount(goCtx, vaulttypes.ModuleName, depositor, minted); err != nil {
        return nil, err
    }

    // 6) update vault accounting in uusdt units
    if err := m.AddDeposit(goCtx, depositor, msg.Units); err != nil {
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

    // 9) log
    fmt.Println(
        "[VAULT] Deposit()",
        "depositor", msg.Creator,
        "deposit_uusdt", msg.Units,
        "minted_uyuu", mintAmt.String(),
        "minted_yuu_human", human,
    )

    return &vaulttypes.MsgDepositResponse{}, nil
}
func (m msgServer) MintFromBridge(
    goCtx context.Context,
    msg *vaulttypes.MsgMintFromBridge,
) (*vaulttypes.MsgMintFromBridgeResponse, error) {
    ctx := sdk.UnwrapSDKContext(goCtx)

    if msg == nil {
        return nil, fmt.Errorf("nil message")
    }
// (A) Bridge operator allow-list (bootstrap on first successful call)
    creatorAddr, err := sdk.AccAddressFromBech32(msg.Creator)
    if err != nil {
        return nil, fmt.Errorf("invalid creator: %w", err)
    }
    if op := m.Keeper.GetBridgeOperator(ctx); op == nil {
        // First successful caller becomes the bridge operator.
        m.Keeper.SetBridgeOperator(ctx, creatorAddr)
    } else if !creatorAddr.Equals(op) {
        return nil, fmt.Errorf("only bridge operator can mint")
    }


    // Parse recipient
    recipient, err := sdk.AccAddressFromBech32(msg.Recipient)
    if err != nil {
        return nil, fmt.Errorf("invalid recipient: %w", err)
    }

    // Build the coin from the integer string and fixed denom uyuu.
    if msg.AmountUyuu == "" {
        return nil, fmt.Errorf("amount_uyuu is required")
    }
    coinStr := msg.AmountUyuu + "uyuu"

    coins, err := sdk.ParseCoinsNormalized(coinStr)
    if err != nil || len(coins) != 1 {
        return nil, fmt.Errorf("invalid amount_uyuu: %s", msg.AmountUyuu)
    }
    coin := coins[0]
    if !coin.Amount.IsPositive() {
        return nil, fmt.Errorf("amount must be positive")
    }

// (B) Optional network allow-list (OFF by default)
    const enforceAllowedNetworks = false
    var allowedNetworks = map[string]struct{}{
        "ethereum": {},
        "sepolia":  {},
    }
    if enforceAllowedNetworks {
        if _, ok := allowedNetworks[strings.ToLower(msg.SourceNetwork)]; !ok {
            return nil, fmt.Errorf("unrecognized source network: %s", msg.SourceNetwork)
        }
    }

    // (C) Optional minimum amount (OFF by default)
    // Set to sdkmath.NewInt(1_000_000_000_000_000) to require >= 0.001 YUU.
    minAmt := sdkmath.ZeroInt()
    if coin.Amount.LT(minAmt) {
        return nil, fmt.Errorf("amount below minimum: %s < %s", coin.Amount.String(), minAmt.String())
    }

    // (D) Idempotency on (source_network, source_tx_hash)
    if m.Keeper.IsSeenBridgeTx(ctx, msg.SourceNetwork, msg.SourceTxHash) {
        return nil, fmt.Errorf("bridge tx already processed: %s/%s", msg.SourceNetwork, msg.SourceTxHash)
    }

    // Mint to module, then send to recipient
    if err := m.Keeper.bankKeeper.MintCoins(ctx, vaulttypes.ModuleName, sdk.NewCoins(coin)); err != nil {
        return nil, err
    }
    if err := m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, vaulttypes.ModuleName, recipient, sdk.NewCoins(coin)); err != nil {
        return nil, err
    }

    // Emit an event for indexers / auditing
    ctx.EventManager().EmitEvent(
        sdk.NewEvent(
            "bridge_mint",
            sdk.NewAttribute("recipient", msg.Recipient),
            sdk.NewAttribute("amount_uyuu", msg.AmountUyuu),
            sdk.NewAttribute("source_network", msg.SourceNetwork),
            sdk.NewAttribute("source_tx_hash", msg.SourceTxHash),
        ),
    )

    m.Keeper.MarkSeenBridgeTx(ctx, msg.SourceNetwork, msg.SourceTxHash)
    return &vaulttypes.MsgMintFromBridgeResponse{}, nil
}
