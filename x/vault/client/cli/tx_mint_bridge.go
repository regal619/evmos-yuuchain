package cli

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"

    "github.com/cosmos/cosmos-sdk/client"
    "github.com/cosmos/cosmos-sdk/client/flags"
    "github.com/cosmos/cosmos-sdk/client/tx"

    vaulttypes "github.com/evmos/evmos/v20/x/vault/types"
)

const (
    FlagRecipient      = "recipient"
    FlagAmountUyuu     = "amount"
    FlagSourceNetwork  = "source-network"
    FlagSourceTxHash   = "source-tx-hash"
    FlagSourceMetadata = "source-metadata"
    FlagMetadataFile   = "metadata-file"
)

func CmdMintFromBridge() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "mint-from-bridge",
        Short: "Mint YUU on YuuChain based on a verified external deposit",
        Args:  cobra.NoArgs,
        RunE: func(cmd *cobra.Command, args []string) error {
            clientCtx, err := client.GetClientTxContext(cmd)
            if err != nil {
                return err
            }

            creator := clientCtx.GetFromAddress().String()
            recipient, _ := cmd.Flags().GetString(FlagRecipient)
            amountUyuu, _ := cmd.Flags().GetString(FlagAmountUyuu)
            sourceNetwork, _ := cmd.Flags().GetString(FlagSourceNetwork)
            sourceTxHash, _ := cmd.Flags().GetString(FlagSourceTxHash)
            sourceMetadata, _ := cmd.Flags().GetString(FlagSourceMetadata)
            metadataFile, _ := cmd.Flags().GetString(FlagMetadataFile)

            if recipient == "" {
                return fmt.Errorf("--%s is required", FlagRecipient)
            }
            if amountUyuu == "" {
                return fmt.Errorf("--%s is required (uyuu, 18-decimal integer)", FlagAmountUyuu)
            }
            if sourceNetwork == "" {
                return fmt.Errorf("--%s is required", FlagSourceNetwork)
            }
            if sourceTxHash == "" {
                return fmt.Errorf("--%s is required", FlagSourceTxHash)
            }

            if metadataFile != "" {
                b, err := os.ReadFile(metadataFile)
                if err != nil {
                    return fmt.Errorf("read --%s: %w", FlagMetadataFile, err)
                }
                sourceMetadata = string(b)
            }

            msg := &vaulttypes.MsgMintFromBridge{
                Creator:            creator,
                Recipient:          recipient,
                AmountUyuu:         amountUyuu,
                SourceNetwork:      sourceNetwork,
                SourceTxHash:       sourceTxHash,
                SourceMetadataJson: sourceMetadata,
            }

            return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
        },
    }

    cmd.Flags().String(FlagRecipient, "", "Recipient bech32 address on YuuChain")
    cmd.Flags().String(FlagAmountUyuu, "", "Amount to mint in uyuu (18-decimal integer, e.g. 1000000000000000000 for 1 YUU)")
    cmd.Flags().String(FlagSourceNetwork, "", "Source network identifier (e.g. sepolia)")
    cmd.Flags().String(FlagSourceTxHash, "", "Source chain tx hash")
    cmd.Flags().String(FlagSourceMetadata, "", "Optional source metadata JSON (string)")
    cmd.Flags().String(FlagMetadataFile, "", "Optional path to a file containing JSON metadata")

    flags.AddTxFlagsToCmd(cmd)
    return cmd
}
