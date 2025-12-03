
package cli

 import (
        "strconv"
        "github.com/spf13/cobra"
        "github.com/cosmos/cosmos-sdk/client/flags"
        "github.com/cosmos/cosmos-sdk/client"
        tx "github.com/cosmos/cosmos-sdk/client/tx"
        vaulttypes "github.com/evmos/evmos/v20/x/vault/types"
 )

 // CmdDeposit defines the deposit command
 func CmdDeposit() *cobra.Command {
         cmd := &cobra.Command{
                Use:   "deposit [units]",
                Short: "Deposit units into the vault (uint64, no denom)",
                 Args:  cobra.ExactArgs(1),
                 RunE: func(cmd *cobra.Command, args []string) error {
                        unitsStr := args[0]

                         clientCtx, err := client.GetClientTxContext(cmd)
                         if err != nil {
                                 return err
                         }

                        // Parse plain uint64 units (no denom)
                        units, err := strconv.ParseUint(unitsStr, 10, 64)
                        if err != nil {
                                return err
                        }

                         msg := &vaulttypes.MsgDeposit{
                                 Creator: clientCtx.GetFromAddress().String(),
                                Units:   units,
                         }

                         return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
                 },
         }
        // Add standard Cosmos SDK tx flags (--from, --fees, etc.)
        flags.AddTxFlagsToCmd(cmd)

        return cmd
 }
