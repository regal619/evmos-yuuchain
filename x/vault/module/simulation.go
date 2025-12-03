//go:build simapp
// This file is included only when building with -tags simapp.
package module

import (
	"math/rand"

	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	vaultsimulation "yuuchain/x/vault/simulation"
        vaulttypes "github.com/evmos/evmos/v20/x/vault/types"
	"yuuchain/x/vault/types"
)

// GenerateGenesisState creates a randomized GenState of the module.
func (AppModule) GenerateGenesisState(simState *module.SimulationState) {
	accs := make([]string, len(simState.Accounts))
	for i, acc := range simState.Accounts {
		accs[i] = acc.Address.String()
	}
	vaultGenesis := vaulttypes.GenesisState{
		Params: vaulttypes.DefaultParams(),
	}
	simState.GenState[vaulttypes.ModuleName] = simState.Cdc.MustMarshalJSON(&vaultGenesis)
}

// RegisterStoreDecoder registers a decoder.
func (am AppModule) RegisterStoreDecoder(_ simtypes.StoreDecoderRegistry) {}

// WeightedOperations returns the all the gov module operations with their respective weights.
func (am AppModule) WeightedOperations(simState module.SimulationState) []simtypes.WeightedOperation {
	operations := make([]simtypes.WeightedOperation, 0)
	const (
		opWeightMsgDeposit          = "op_weight_msg_vault"
		defaultWeightMsgDeposit int = 100
	)

	var weightMsgDeposit int
	simState.AppParams.GetOrGenerate(opWeightMsgDeposit, &weightMsgDeposit, nil,
		func(_ *rand.Rand) {
			weightMsgDeposit = defaultWeightMsgDeposit
		},
	)
	operations = append(operations, simulation.NewWeightedOperation(
		weightMsgDeposit,
		vaultsimulation.SimulateMsgDeposit(am.authKeeper, am.bankKeeper, am.keeper, simState.TxConfig),
	))

	return operations
}

// ProposalMsgs returns msgs used for governance proposals for simulations.
func (am AppModule) ProposalMsgs(simState module.SimulationState) []simtypes.WeightedProposalMsg {
	return []simtypes.WeightedProposalMsg{}
}
