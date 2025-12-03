// x/vault/client/cli/query_yuu_balance.go
package cli

import (
    "context"
    "fmt"
    "math/big"

    "github.com/spf13/cobra"

    "github.com/cosmos/cosmos-sdk/client"
    sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
    banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

// CmdYuuBalance shows the account's Yuu balance in human form (YUU) as well as raw uyuu.
func CmdYuuBalance() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "yuu-balance [address]",
        Short: "Show Yuu balance for an address (raw uyuu + human YUU)",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            addr := args[0]

            clientCtx, err := client.GetClientQueryContext(cmd)
            if err != nil {
                return err
            }

            // query bank balances
            bq := banktypes.NewQueryClient(clientCtx)
            res, err := bq.AllBalances(context.Background(), &banktypes.QueryAllBalancesRequest{
                Address: addr,
            })
            if err != nil {
                return err
            }

            // find the uyuu coin
            var uyuuAmt string
            for _, c := range res.Balances {
                if c.Denom == "uyuu" {
                    uyuuAmt = c.Amount.String()
                    break
                }
            }

            // print even if zero
            fmt.Println("address:", addr)
            fmt.Println("raw_uyuu:", uyuuAmt)

            if uyuuAmt == "" {
                fmt.Println("human_yuu:", "0")
                return nil
            }

            // human = uyuu / 1e18
            // we'll do it with big.Rat so we keep precision
            uyuuInt := new(big.Int)
            uyuuInt, ok := uyuuInt.SetString(uyuuAmt, 10)
            if !ok {
                return fmt.Errorf("cannot parse uyuu amount %s", uyuuAmt)
            }

            scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil) // 1e18
            rat := new(big.Rat).SetFrac(uyuuInt, scale)

            // format nicely, say 18 decimals max
            humanStr := rat.FloatString(18)

            fmt.Println("human_yuu:", humanStr)

            return nil
        },
    }

    sdkflags.AddQueryFlagsToCmd(cmd)
    return cmd
}
