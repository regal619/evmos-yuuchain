package cli

import "github.com/spf13/cobra"

// GetTxCmd returns the root tx command for the vault module.
func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "vault",
		Short:                      "Vault transaction subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}

	cmd.AddCommand(CmdDeposit())
        cmd.AddCommand(CmdMintFromBridge())
        cmd.AddCommand(CmdMintFromBridge())
	return cmd
}
