package cli

import (
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	vaulttypes "github.com/evmos/evmos/v20/x/vault/types"
)

const (
	FlagRecipient          = "recipient"
	FlagAmountUyuu         = "amount-uyuu"
	FlagSourceNetwork      = "source-network"
	FlagSourceTxHash       = "source-tx-hash"
	FlagSourceMetadata     = "source-metadata"
	FlagSourceMetadataFile = "source-metadata-file"
        FlagSourceAmountUusdt = "source-amount-uusdt"
)

func CmdMintFromBridge() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mint-from-bridge",
		Short: "Mint YUU to a recipient based on a verified external deposit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			recipient, _ := cmd.Flags().GetString(FlagRecipient)
			amountUyuu, _ := cmd.Flags().GetString(FlagAmountUyuu)
			srcNet, _ := cmd.Flags().GetString(FlagSourceNetwork)
			srcHash, _ := cmd.Flags().GetString(FlagSourceTxHash)
			meta, _ := cmd.Flags().GetString(FlagSourceMetadata)
			metaFile, _ := cmd.Flags().GetString(FlagSourceMetadataFile)
                        sourceAmountUusdt, _ := cmd.Flags().GetUint64(FlagSourceAmountUusdt)

			if metaFile != "" {
				b, err := os.ReadFile(metaFile)
				if err != nil {
					return err
				}
				meta = string(b)
			}

			msg := &vaulttypes.MsgMintFromBridge{
				Creator:            clientCtx.GetFromAddress().String(),
				Recipient:          recipient,
				AmountUyuu:         amountUyuu,
				SourceNetwork:      srcNet,
				SourceTxHash:       srcHash,
				SourceMetadataJson: meta,
                                SourceAmountUusdt: sourceAmountUusdt,
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	cmd.Flags().String(FlagRecipient, "", "YuuChain recipient bech32 address")
	cmd.Flags().String(FlagAmountUyuu, "", "Amount to mint in uyuu (1e18 precision)")
        cmd.Flags().String(FlagSourceNetwork, "", "Source network ID (e.g. \"sepolia\")")
	cmd.Flags().String(FlagSourceTxHash, "", "Source chain transaction hash")
	cmd.Flags().String(FlagSourceMetadata, "", "Optional JSON metadata (inline)")
	cmd.Flags().String(FlagSourceMetadataFile, "", "Optional path to JSON metadata file")
        cmd.Flags().Uint64(FlagSourceAmountUusdt, 0, "Amount of externally locked USDT in uusdt (1e6 = 1 USDT)",
)

	_ = cmd.MarkFlagRequired(FlagRecipient)
	_ = cmd.MarkFlagRequired(FlagAmountUyuu)
	_ = cmd.MarkFlagRequired(FlagSourceNetwork)
	_ = cmd.MarkFlagRequired(FlagSourceTxHash)

	sdkflags.AddTxFlagsToCmd(cmd)
	return cmd
}
