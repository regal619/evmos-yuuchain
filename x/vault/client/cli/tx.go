package cli

import (
	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"

	vaulttypes "github.com/evmos/evmos/v20/x/vault/types"
)

// GetTxCmd returns the root tx command for the vault module.
// It only wires commands that actually exist in this folder.
func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        vaulttypes.ModuleName,
		Short:                      "Transaction commands for the vault module",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	// Add tx subcommands that exist.
	cmd.AddCommand(
		GetCmdDeposit(),      // from tx_deposit.go (already in your folder)
		CmdMintFromBridge(),  // from tx_mint_from_bridge.go
	)

	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
