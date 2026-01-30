package keeper

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
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
	// (A) Bridge operator allow-list (PRODUCTION LOCK)
	creatorAddr, err := sdk.AccAddressFromBech32(msg.Creator)
	if err != nil {
		return nil, fmt.Errorf("invalid creator: %w", err)
	}

	const bridgeOperatorBech32 = "evmos1xgeppskksl5g6fnvzlnaun4xnql8f8gdr5dsz7"

	op := m.Keeper.GetBridgeOperator(ctx)
	if op == nil {
		fixedOp, err := sdk.AccAddressFromBech32(bridgeOperatorBech32)
		if err != nil {
			return nil, fmt.Errorf("invalid fixed bridge operator address: %w", err)
		}
		m.Keeper.SetBridgeOperator(ctx, fixedOp)
		op = fixedOp
	}

	if !creatorAddr.Equals(op) {
		return nil, fmt.Errorf("only bridge operator can mint")
	}

	// Parse recipient
	recipient, err := sdk.AccAddressFromBech32(msg.Recipient)
	if err != nil {
		return nil, fmt.Errorf("invalid recipient: %w", err)
	}

	// Require source_amount_uusdt so the chain can compute mint amount (Model B).
	if msg.SourceAmountUusdt == 0 {
		return nil, fmt.Errorf("source_amount_uusdt must be provided")
	}

	// Compute mint amount on-chain using the same bonding-curve integrator as Deposit().
	// IMPORTANT: compute BEFORE adding the new liquidity so we integrate from D0 -> D1 correctly.
	mintAmt, err := m.Keeper.MintAmountForDeposit(goCtx, msg.SourceAmountUusdt)
	if err != nil {
		return nil, err
	}
	if !mintAmt.IsPositive() {
		return nil, fmt.Errorf("bridge deposit too small for current Yuu price")
	}

	coin := sdk.NewCoin("uyuu", mintAmt)

	// (B) Option B: On-chain network allow-list (params).
	// Backwards-compatible behavior:
	// - if AllowedSourceNetworks is empty => allow all networks
	// - if non-empty => require msg.SourceNetwork to be in the list
	params := m.Keeper.GetParams(ctx)

	if len(params.AllowedSourceNetworks) > 0 {
		sn := strings.ToLower(strings.TrimSpace(msg.SourceNetwork))
		allowed := false
		for _, n := range params.AllowedSourceNetworks {
			if sn == strings.ToLower(strings.TrimSpace(n)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("unrecognized source network: %s", msg.SourceNetwork)
		}
	}

	// (B2) Token allowlist per network (USDT/USDC contracts).
	// Backwards-compatible behavior:
	// - if AllowedTokens is empty => do not enforce token-level restrictions
	// - if non-empty => require (source_network, token_symbol, token_address) to match an entry
	if len(params.AllowedTokens) > 0 {
		sym := strings.ToUpper(strings.TrimSpace(msg.TokenSymbol))
		if sym == "" {
			return nil, fmt.Errorf("token_symbol is required")
		}

		addr := strings.TrimSpace(msg.TokenAddress)
		if addr == "" {
			return nil, fmt.Errorf("token_address is required")
		}

		sn := strings.ToLower(strings.TrimSpace(msg.SourceNetwork))

		// Normalize address for EVM comparisons (0x...): compare lowercase.
		// For non-EVM (e.g. Tron base58), keep as-is (case-sensitive).
		normAddr := addr
		if strings.HasPrefix(strings.ToLower(addr), "0x") {
			normAddr = strings.ToLower(addr)
		}

		ok := false
		for _, t := range params.AllowedTokens {
			if t == nil {
				continue
			}
			tsn := strings.ToLower(strings.TrimSpace(t.SourceNetwork))
			tsym := strings.ToUpper(strings.TrimSpace(t.TokenSymbol))
			taddr := strings.TrimSpace(t.TokenAddress)
			if strings.HasPrefix(strings.ToLower(taddr), "0x") {
				taddr = strings.ToLower(taddr)
			}

			if sn == tsn && sym == tsym && normAddr == taddr {
				ok = true
				break
			}
		}

		if !ok {
			return nil, fmt.Errorf(
				"unsupported token: network=%s symbol=%s address=%s",
				msg.SourceNetwork, msg.TokenSymbol, msg.TokenAddress,
			)
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

	// (E) CREDIT VAULT FROM BRIDGE (native uusdt accounting)
	m.Keeper.AddVaultLiquidityFromBridge(goCtx, msg.SourceAmountUusdt)

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
			sdk.NewAttribute("amount_uyuu", coin.Amount.String()),
			sdk.NewAttribute("source_network", msg.SourceNetwork),
			sdk.NewAttribute("source_tx_hash", msg.SourceTxHash),
			sdk.NewAttribute("token_symbol", msg.TokenSymbol),
			sdk.NewAttribute("token_address", msg.TokenAddress),
			sdk.NewAttribute("source_amount_uusdt", fmt.Sprintf("%d", msg.SourceAmountUusdt)),
		),
	)

	m.Keeper.MarkSeenBridgeTx(ctx, msg.SourceNetwork, msg.SourceTxHash)
	return &vaulttypes.MsgMintFromBridgeResponse{}, nil
}
