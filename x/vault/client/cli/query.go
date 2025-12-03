package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"

        vaulttypes "github.com/evmos/evmos/v20/x/vault/types"
)

// GetQueryCmd returns the parent "vault" query command.
func GetQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vault",
		Short: "Vault module queries",
	}

	cmd.AddCommand(
		CmdParams(),
		CmdQueryDeposit(),
                CmdQueryTotal(),
                CmdPrice(),
                CmdYuuBalance(),
                CmdVaultSnapshot(),
	)

	return cmd
}

// CmdParams queries the vault module params.
func CmdParams() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "params",
		Short: "Query vault module params",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			q := vaulttypes.NewQueryClient(clientCtx)
			res, err := q.Params(context.Background(), &vaulttypes.QueryParamsRequest{})
			if err != nil {
				return err
			}
			return clientCtx.PrintProto(res)
		},
	}
	sdkflags.AddQueryFlagsToCmd(cmd)
	return cmd
}

// CmdDeposit queries the recorded deposit for a given address.
func CmdQueryDeposit() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deposit [address]",
		Short: "Query recorded deposit amount for an address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			addr := args[0]

			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			q := vaulttypes.NewQueryClient(clientCtx)
			res, err := q.Deposit(context.Background(), &vaulttypes.DepositRequest{
				Address: addr,
			})
			if err != nil {
				return err
			}
			return clientCtx.PrintProto(res)
		},
	}
	sdkflags.AddQueryFlagsToCmd(cmd)
	return cmd
}
// CmdQueryTotal queries the global total deposits recorded by the vault.
func CmdQueryTotal() *cobra.Command {
        cmd := &cobra.Command{
                Use:   "total",
                Short: "Query total recorded deposits",
                Args:  cobra.NoArgs,
                RunE: func(cmd *cobra.Command, _ []string) error {
                        clientCtx, err := client.GetClientQueryContext(cmd)
                        if err != nil {
                                return err
                        }
                        q := vaulttypes.NewQueryClient(clientCtx)
                        res, err := q.Total(context.Background(), &vaulttypes.TotalRequest{})
                        if err != nil {
                                return err
                        }
                        return clientCtx.PrintProto(res)
                },
        }
        sdkflags.AddQueryFlagsToCmd(cmd)
        return cmd
}

func CmdPrice() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "price",
        Short: "show current Yuu price (uusdt per 1 yuu)",
        Args:  cobra.NoArgs,
        RunE: func(cmd *cobra.Command, args []string) error {
            clientCtx, err := client.GetClientQueryContext(cmd)
            if err != nil {
                return err
            }

            q := vaulttypes.NewQueryClient(clientCtx)

            res, err := q.Price(context.Background(), &vaulttypes.PriceRequest{})
            if err != nil {
                return err
            }

            return clientCtx.PrintProto(res)
        },
    }

    sdkflags.AddQueryFlagsToCmd(cmd)
    return cmd
}
