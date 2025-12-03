//go:build simapp
// This simulation helper is only built when -tags simapp is provided.
package simulation

import (
	"math/rand"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"

        vaultkeeper   "github.com/evmos/evmos/v20/x/vault/keeper"
        vaulttypes    "github.com/evmos/evmos/v20/x/vault/types"
)

func SimulateMsgDeposit(
	ak vaulttypes.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accs)
		msg := &vaulttypes.MsgDeposit{
			Creator: simAccount.Address.String(),
		}

		// TODO: Handle the Deposit simulation

		return simtypes.NoOpMsg(vaulttypes.ModuleName, sdk.MsgTypeURL(msg), "Deposit simulation not implemented"), nil, nil
	}
}
