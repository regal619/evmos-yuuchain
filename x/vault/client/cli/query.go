package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"

	vaulttypes "github.com/evmos/evmos/v20/x/vault/types"
)

func GetQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        vaulttypes.ModuleName,
		Short:                      "Query commands for the vault module",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
                GetCmdParams(),
		GetCmdTotal(),
                GetCmdExternal(),
                GetCmdExternalByNetwork(),
		GetCmdPrice(),
		GetCmdDeposit(),
                GetCmdMyDeposit(),
                GetCmdBridgeOperator(),
	)

	return cmd
}

func GetCmdParams() *cobra.Command {
        cmd := &cobra.Command{
                Use:   "params",
                Short: "Query the vault module parameters",
                Args:  cobra.NoArgs,
                RunE: func(cmd *cobra.Command, _ []string) error {
                        cctx, err := client.GetClientQueryContext(cmd)
                        if err != nil {
                                return err
                        }

                        q := vaulttypes.NewQueryClient(cctx)
                        res, err := q.Params(context.Background(), &vaulttypes.QueryParamsRequest{})
                        if err != nil {
                                return err
                        }

                        return cctx.PrintProto(res)
                },
        }

        flags.AddQueryFlagsToCmd(cmd)
        return cmd
}

func GetCmdTotal() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "total",
		Short: "Query total deposited amount across all accounts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cctx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			q := vaulttypes.NewQueryClient(cctx)
			res, err := q.Total(context.Background(), &vaulttypes.TotalRequest{})
			if err != nil {
				return err
			}

			return cctx.PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)
	return cmd
}

func GetCmdPrice() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "price",
		Short: "Query current YUU price",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cctx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			q := vaulttypes.NewQueryClient(cctx)
			res, err := q.Price(context.Background(), &vaulttypes.PriceRequest{})
			if err != nil {
				return err
			}

			return cctx.PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)
	return cmd
}

func GetCmdDeposit() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deposit [address]",
		Short: "Query deposit amount for an address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			addr := args[0]

			cctx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			q := vaulttypes.NewQueryClient(cctx)
			res, err := q.Deposit(context.Background(), &vaulttypes.DepositRequest{Address: addr})
			if err != nil {
				return err
			}

			return cctx.PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)
	return cmd
}


       func GetCmdMyDeposit() *cobra.Command {
        cmd := &cobra.Command{
                Use:   "my-deposit",
                Short: "Query deposit amount for the --from address",
                Args:  cobra.NoArgs,
                RunE: func(cmd *cobra.Command, _ []string) error {
                // We use TxContext here so --from can resolve key names from the keyring.
                cctx, err := client.GetClientTxContext(cmd)
                if err != nil {
                return err
                }
                        fromAddr := cctx.GetFromAddress().String()

                        q := vaulttypes.NewQueryClient(cctx)
                        res, err := q.Deposit(context.Background(), &vaulttypes.DepositRequest{
                                Address: fromAddr,
                        })
                         if err != nil {
                                  return err
                          }

                        return cctx.PrintProto(res)
                },
        }

        flags.AddQueryFlagsToCmd(cmd)
        return cmd
}
        func GetCmdBridgeOperator() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bridge-operator",
		Short: "Query the currently authorized bridge operator address",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cctx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			q := vaulttypes.NewQueryClient(cctx)
			res, err := q.BridgeOperator(
				context.Background(),
				&vaulttypes.QueryBridgeOperatorRequest{},
			)
			if err != nil {
				return err
			}

			return cctx.PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)
	return cmd
}
        func GetCmdExternal() *cobra.Command {
        cmd := &cobra.Command{
                Use:   "external",
                Short: "Query total externally locked USDT (USDT.yuu)",
                Args:  cobra.NoArgs,
                RunE: func(cmd *cobra.Command, _ []string) error {
                        cctx, err := client.GetClientQueryContext(cmd)
                        if err != nil {
                                return err
                        }

                        q := vaulttypes.NewQueryClient(cctx)
                        res, err := q.External(context.Background(), &vaulttypes.TotalRequest{})
                        if err != nil {
                                return err
                        }

                        return cctx.PrintProto(res)
                },
        }

        flags.AddQueryFlagsToCmd(cmd)
        return cmd
}
func GetCmdExternalByNetwork() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "external-by-network [network]",
		Short: "Query externally locked USDT for a specific source network",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			network := args[0]

			cctx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			q := vaulttypes.NewQueryClient(cctx)
			res, err := q.ExternalByNetwork(
				context.Background(),
				&vaulttypes.ExternalByNetworkRequest{
					Network: network,
				},
			)
			if err != nil {
				return err
			}

			return cctx.PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)
	return cmd
}
