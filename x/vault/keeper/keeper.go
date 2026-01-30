// x/vault/keeper/keeper.go
package keeper

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strings"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	vaulttypes "github.com/evmos/evmos/v20/x/vault/types"
)

type Keeper struct {
	// Embed the unimplemented Query server so *Keeper satisfies types.QueryServer.
	vaulttypes.UnimplementedQueryServer

	storeService corestore.KVStoreService
	cdc          codec.Codec
	addressCodec address.Codec
	authority    []byte

	// bank access to move coins into the vault
	bankKeeper vaulttypes.BankKeeper

	// NOTE: Rename to avoid clashing with the Query method name "Params".
	ParamsItem collections.Item[vaulttypes.Params]
	Schema     collections.Schema
}

func NewKeeper(
	storeService corestore.KVStoreService,
	cdc codec.Codec,
	addressCodec address.Codec,
	authority []byte,
	bankKeeper vaulttypes.BankKeeper,
) *Keeper {
	sb := collections.NewSchemaBuilder(storeService)

	k := &Keeper{
		storeService: storeService,
		cdc:          cdc,
		addressCodec: addressCodec,
		authority:    authority,
		bankKeeper:   bankKeeper,
		ParamsItem:   collections.NewItem(sb, vaulttypes.ParamsKey, "params", codec.CollValue[vaulttypes.Params](cdc)),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema

	return k
}

// ---- helpers for params (thin wrappers over the collections item) ----
func (k Keeper) GetParams(ctx sdk.Context) vaulttypes.Params {
	p, err := k.ParamsItem.Get(ctx)
	if err != nil {
		// nothing stored → just use whatever the module’s default is
		return vaulttypes.DefaultParams()
	}
	return p
}

func (k Keeper) SetParams(ctx sdk.Context, p vaulttypes.Params) {
	if err := k.ParamsItem.Set(ctx, p); err != nil {
		panic(err)
	}
}

// ---------------------------------------------------------------------

func AddressKey(addr sdk.AccAddress) []byte {
	return append(vaulttypes.KeyPrefixDeposits, addr.Bytes()...)
}

func (k Keeper) getInt(store corestore.KVStore, key []byte) sdkmath.Int {
	if bz, _ := store.Get(key); bz != nil {
		return sdkmath.NewIntFromBigInt(new(big.Int).SetBytes(bz))
	}
	return sdkmath.ZeroInt()
}

func (k Keeper) setInt(store corestore.KVStore, key []byte, v sdkmath.Int) {
	_ = store.Set(key, v.BigInt().Bytes())
}

// AddVaultLiquidityFromBridge increases the vault's native USDT total.
// This is accounting-only and may ONLY be called by the bridge path.
func (k Keeper) AddVaultLiquidityFromBridge(ctx context.Context, amount uint64) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	kv := k.storeService.OpenKVStore(sdkCtx)

	amt := sdkmath.NewIntFromUint64(amount)

	curTotal := k.getInt(kv, vaulttypes.KeyTotalDeposits)
	k.setInt(kv, vaulttypes.KeyTotalDeposits, curTotal.Add(amt))
}

// AddDeposit increments <addr>'s deposit and the global total by <amount>.
func (k Keeper) AddDeposit(ctx context.Context, addr sdk.AccAddress, amount uint64) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	kv := k.storeService.OpenKVStore(sdkCtx)

	amt := sdkmath.NewIntFromUint64(amount)

	// per-address
	addrKey := AddressKey(addr)
	cur := k.getInt(kv, addrKey)
	k.setInt(kv, addrKey, cur.Add(amt))

	// global
	totalKey := vaulttypes.KeyTotalDeposits
	curTotal := k.getInt(kv, totalKey)
	k.setInt(kv, totalKey, curTotal.Add(amt))

	return nil
}

func (k Keeper) GetDeposit(ctx context.Context, addr sdk.AccAddress) sdkmath.Int {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	kv := k.storeService.OpenKVStore(sdkCtx)
	return k.getInt(kv, AddressKey(addr))
}

func (k Keeper) GetTotal(ctx context.Context) sdkmath.Int {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	kv := k.storeService.OpenKVStore(sdkCtx)
	return k.getInt(kv, vaulttypes.KeyTotalDeposits)
}

// Example of using errorsmod (kept so the import is useful if you need it later)
var _ = errorsmod.Wrap

func (k Keeper) GetAuthority() string {
	s, _ := k.addressCodec.BytesToString(k.authority)
	return s
}

// --- Bridge operator allow-list (single address) ---
// Stored as raw AccAddress bytes under BridgeOperatorKey.
func (k Keeper) GetBridgeOperator(ctx sdk.Context) sdk.AccAddress {
	kv := k.storeService.OpenKVStore(ctx)
	bz, _ := kv.Get(vaulttypes.BridgeOperatorKey)
	if len(bz) == 0 {
		return nil
	}
	return sdk.AccAddress(bz)
}

func (k Keeper) SetBridgeOperator(ctx sdk.Context, addr sdk.AccAddress) {
	kv := k.storeService.OpenKVStore(ctx)
	_ = kv.Set(vaulttypes.BridgeOperatorKey, addr)
}

// BridgeOperator is a read-only gRPC query that returns the currently authorized bridge operator.
// This exposes the same authority enforced by MsgMintFromBridge.
func (k Keeper) BridgeOperator(ctx context.Context, _ *vaulttypes.QueryBridgeOperatorRequest) (*vaulttypes.QueryBridgeOperatorResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	op := k.GetBridgeOperator(sdkCtx)
	if op == nil {
		return &vaulttypes.QueryBridgeOperatorResponse{Operator: ""}, nil
	}
	return &vaulttypes.QueryBridgeOperatorResponse{Operator: op.String()}, nil
}

// --- Idempotency on (source_network, source_tx_hash) ---
func (k Keeper) seenBridgeTxKey(network, txhash string) []byte {
	key := fmt.Sprintf("%s%s|%s", string(vaulttypes.SeenBridgeTxPrefix), strings.ToLower(network), strings.ToLower(txhash))
	return []byte(key)
}

func (k Keeper) IsSeenBridgeTx(ctx sdk.Context, network, txhash string) bool {
	kv := k.storeService.OpenKVStore(ctx)
	bz, _ := kv.Get(k.seenBridgeTxKey(network, txhash))
	return bz != nil
}

func (k Keeper) MarkSeenBridgeTx(ctx sdk.Context, network, txhash string) {
	kv := k.storeService.OpenKVStore(ctx)
	_ = kv.Set(k.seenBridgeTxKey(network, txhash), []byte{1})
}

// GetYuuPrice returns the bonding-curve price of 1 YUU, but we store
// deposits on-chain in uusdt, so we must convert to USDT (÷ 1e6) before
// sending it to the anchor curve.
func (k Keeper) GetYuuPrice(ctx context.Context) sdkmath.LegacyDec {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	store := k.storeService.OpenKVStore(sdkCtx)

	// total deposits as stored: uusdt
	totalDepositsUusdt := k.getInt(store, vaulttypes.KeyTotalDeposits)

	// turn into float
	dFloat, _ := new(big.Float).SetInt(totalDepositsUusdt.BigInt()).Float64()

	// convert uusdt → USDT for the curve
	dUsdt := dFloat / 1_000_000.0

	// now run the curve on USDT
	price := priceFromAnchors(dUsdt)

	// return as 18-decimal string dec
	return sdkmath.LegacyMustNewDecFromStr(fmt.Sprintf("%.18f", price))
}

// priceFromAnchors takes a deposit amount D (uusdt) and returns the price
// by interpolating between anchor points. Above the last anchor, it extends
// with the last segment's slope.
func priceFromAnchors(D float64) float64 {
	// deposit anchors (uusdt)
	dep := []float64{
		0,
		1_000,
		10_000,
		100_000,
		1_000_000,
		10_000_000,
		100_000_000,
		1_000_000_000,
		10_000_000_000,
		100_000_000_000,
	}
	// matching prices
	prc := []float64{
		200.00,
		200.86,
		202.98,
		205.81,
		213.11,
		265.66,
		415.18,
		678.19,
		1471.78,
		4693.65,
	}

	// no deposits yet
	if D <= dep[0] {
		return prc[0]
	}

	n := len(dep)
	// inside the table
	for i := 0; i < n-1; i++ {
		d0 := dep[i]
		d1 := dep[i+1]
		if D >= d0 && D <= d1 {
			p0 := prc[i]
			p1 := prc[i+1]

			// first segment has d0 = 0 → use linear
			if d0 == 0 {
				frac := D / d1
				return p0 + frac*(p1-p0)
			}

			// power-law interpolation: p = p0 * (D/d0)^s
			s := math.Log(p1/p0) / math.Log(d1/d0)
			return p0 * math.Pow(D/d0, s)
		}
	}

	// above last anchor → extend using last segment slope
	d0 := dep[n-2]
	d1 := dep[n-1]
	p0 := prc[n-2]
	p1 := prc[n-1]

	s := math.Log(p1/p0) / math.Log(d1/d0)
	return p1 * math.Pow(D/d1, s)
}

// bondingPriceFloat is what the integrator uses.
func (k Keeper) bondingPriceFloat(D float64) float64 {
	return priceFromAnchors(D)
}

// integrate1OverPrice does a simple trapezoid integration of 1/P(x)
// from a -> b using 'steps' slices.
func (k Keeper) integrate1OverPrice(a, b float64, steps int) float64 {
	if b <= a {
		return 0
	}
	h := (b - a) / float64(steps)
	sum := 0.0
	x := a
	for i := 0; i <= steps; i++ {
		p := k.bondingPriceFloat(x)
		if p <= 0 {
			p = 1 // safety
		}
		weight := 1.0
		if i == 0 || i == steps {
			weight = 0.5
		}
		sum += weight * (1.0 / p)
		x += h
	}
	return sum * h
}

// MintAmountForDeposit numerically integrates how many Yuu should be
// minted when a new deposit of deposit (in uusdt) arrives.
//
// IMPORTANT: this returns the amount ALREADY SCALED to 18 decimals.
// i.e. if the math says "mint 3.456 YUU", this will return 3456000000000000000.
func (k Keeper) MintAmountForDeposit(ctx context.Context, deposit uint64) (sdkmath.Int, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	store := k.storeService.OpenKVStore(sdkCtx)

	// current total deposits, as stored on-chain (in uusdt)
	curTotalUusdt := k.getInt(store, vaulttypes.KeyTotalDeposits)

	// convert current total to REAL USDT for the curve
	curTotalFloat, _ := new(big.Float).SetInt(curTotalUusdt.BigInt()).Float64()
	D0 := curTotalFloat / 1_000_000.0 // -> USDT

	// new deposit also arrives in uusdt → convert to USDT
	depUSDT := float64(deposit) / 1_000_000.0
	D1 := D0 + depUSDT

	// integrate 1 / price(x) from D0 to D1 in USDT space
	yuuFloat := k.integrate1OverPrice(D0, D1, 200)
	if yuuFloat <= 0 {
		return sdkmath.ZeroInt(), nil
	}

	// we want to mint fractional Yuu on-chain → scale to 1e18 uyuu
	f := new(big.Float).Mul(big.NewFloat(yuuFloat), big.NewFloat(1e18))
	scaled, _ := f.Int(nil)

	return sdkmath.NewIntFromBigInt(scaled), nil
}
