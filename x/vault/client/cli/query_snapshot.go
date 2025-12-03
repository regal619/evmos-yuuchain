package cli

import (
	"context"
	"fmt"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/client"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"

        vaulttypes "github.com/evmos/evmos/v20/x/vault/types"
)

// CmdVaultSnapshot shows a quick summary of vault state:
// - total deposits (uusdt and USDT)
// - current Yuu price
func CmdVaultSnapshot() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Show vault high-level snapshot (total, price)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			q := vaulttypes.NewQueryClient(clientCtx)

			// 1) total deposits (this is in uusdt string)
			totalRes, err := q.Total(context.Background(), &vaulttypes.TotalRequest{})
			if err != nil {
				return err
			}

			// parse total uusdt safely
			totalUusdt, ok := sdkmath.NewIntFromString(totalRes.Total)
			if !ok {
				totalUusdt = sdkmath.ZeroInt()
			}
			// convert to USDT (1e6)
			totalUsdt := float64(totalUusdt.Int64()) / 1_000_000.0

			// 2) current price
			priceRes, err := q.Price(context.Background(), &vaulttypes.PriceRequest{})
			if err != nil {
				return err
			}

			// print using clientCtx so it never panics
			out := fmt.Sprintf(
				"vault snapshot:\n  total_deposits_uusdt: %s\n  total_deposits_usdt: %.6f\n  current_yuu_price_usdt: %s\n",
				totalUusdt.String(),
				totalUsdt,
				priceRes.Price,
			)

			return clientCtx.PrintString(out)
		},
	}

	sdkflags.AddQueryFlagsToCmd(cmd)
	return cmd
}
