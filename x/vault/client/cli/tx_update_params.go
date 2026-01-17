package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	vaulttypes "github.com/evmos/evmos/v20/x/vault/types"
)

// CmdUpdateParams adds a minimal operator CLI to update vault params on-chain.
// It reads the CURRENT on-chain params first, then applies additive changes
// (add networks, add allowed tokens), and submits MsgUpdateParams.
func CmdUpdateParams() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params",
		Short: "Update vault params (add allowed source networks / tokens)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			txCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			addNetworks, _ := cmd.Flags().GetStringArray("add-source-network")
			addTokens, _ := cmd.Flags().GetStringArray("add-token")
			authorityFlag, _ := cmd.Flags().GetString("authority")

			authority := authorityFlag
			if strings.TrimSpace(authority) == "" {
				authority = txCtx.GetFromAddress().String()
			}

			// 1) Query current params via the same binary (avoids grpc wiring here)
			out, err := exec.Command(
				os.Args[0],
				"query", "vault", "params",
				"--node", txCtx.NodeURI,
				"-o", "json",
			).CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to query current params: %v\n%s", err, string(out))
			}

			j := parseLastJSONObject(string(out))
			if j == nil {
				return fmt.Errorf("could not parse JSON from query output:\n%s", string(out))
			}

			paramsObj, ok := j["params"].(map[string]any)
			if !ok {
				return fmt.Errorf("unexpected params query shape (missing .params):\n%s", string(out))
			}

			// 2) Build Params from the query JSON
			p := vaulttypes.Params{}

			// allowed_source_networks
			if arr, ok := paramsObj["allowed_source_networks"].([]any); ok {
				for _, v := range arr {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						p.AllowedSourceNetworks = append(p.AllowedSourceNetworks, s)
					}
				}
			}

			// allowed_tokens
			if arr, ok := paramsObj["allowed_tokens"].([]any); ok {
				for _, it := range arr {
					m, ok := it.(map[string]any)
					if !ok {
						continue
					}
					sn, _ := m["source_network"].(string)
					sym, _ := m["token_symbol"].(string)
					addr, _ := m["token_address"].(string)
					if strings.TrimSpace(sn) == "" || strings.TrimSpace(sym) == "" || strings.TrimSpace(addr) == "" {
						continue
					}
					p.AllowedTokens = append(p.AllowedTokens, &vaulttypes.AllowedToken{
						SourceNetwork: sn,
						TokenSymbol:   sym,
						TokenAddress:  addr,
					})
				}
			}

			// 3) Apply additive changes (no removals)
			for _, n := range addNetworks {
				n = strings.TrimSpace(n)
				if n == "" {
					continue
				}
				if !containsString(p.AllowedSourceNetworks, n) {
					p.AllowedSourceNetworks = append(p.AllowedSourceNetworks, n)
				}
			}

			// --add-token format: source_network:token_symbol:token_address
			for _, t := range addTokens {
				t = strings.TrimSpace(t)
				if t == "" {
					continue
				}
				parts := strings.SplitN(t, ":", 3)
				if len(parts) != 3 {
					return fmt.Errorf("bad --add-token %q (expected source_network:token_symbol:token_address)", t)
				}
				sn := strings.TrimSpace(parts[0])
				sym := strings.TrimSpace(parts[1])
				addr := strings.TrimSpace(parts[2])
				if sn == "" || sym == "" || addr == "" {
					return fmt.Errorf("bad --add-token %q (empty field)", t)
				}
				if !containsAllowedToken(p.AllowedTokens, sn, sym, addr) {
					p.AllowedTokens = append(p.AllowedTokens, &vaulttypes.AllowedToken{
						SourceNetwork: sn,
						TokenSymbol:   sym,
						TokenAddress:  addr,
					})
				}
			}

			// 4) Build and broadcast MsgUpdateParams
			//
			// NOTE: Your generated type should exist if the chain supports update params.
			// If it doesn’t compile, it means MsgUpdateParams is not in your proto/keeper yet.
			msg := &vaulttypes.MsgUpdateParams{
				Authority: authority,
				Params:    &p,
			}


			return tx.GenerateOrBroadcastTxCLI(txCtx, cmd.Flags(), msg)
		},
	}

	cmd.Flags().StringArray("add-source-network", nil, "Add an allowed source network (repeatable), e.g. tron")
	cmd.Flags().StringArray("add-token", nil, "Add an allowed token (repeatable): source_network:token_symbol:token_address")
	cmd.Flags().String("authority", "", "Authority address for MsgUpdateParams (default: --from address)")

	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func containsString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func containsAllowedToken(xs []*vaulttypes.AllowedToken, sn, sym, addr string) bool {
	for _, x := range xs {
		if x == nil {
			continue
		}
		if x.SourceNetwork == sn && x.TokenSymbol == sym && x.TokenAddress == addr {
			return true
		}
	}
	return false
}

// parseLastJSONObject handles noisy stdout (module prints before JSON) by extracting the last JSON object.
func parseLastJSONObject(noisy string) map[string]any {
	text := strings.TrimSpace(noisy)
	if text == "" {
		return nil
	}
	for i := len(text) - 1; i >= 0; i-- {
		if text[i] != '{' {
			continue
		}
		cand := text[i:]
		var obj map[string]any
		if err := json.Unmarshal([]byte(cand), &obj); err == nil {
			return obj
		}
	}
	return nil
}
