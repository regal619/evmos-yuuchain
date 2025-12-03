package module

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
)

// AutoCLIOptions implements the autocli.HasAutoCLIConfig interface.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			// Use FQN strings; do NOT reference generated *_ServiceDesc
			Service: "yuuchain.vault.v1.Query",
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Shows the parameters of the module",
				},
				{
					RpcMethod: "Deposit",
					Use:       "deposit [address]",
					Short:     "Query deposit amount for an address",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "address"},
					},
				},
				{
					RpcMethod: "Total",
					Use:       "total",
					Short:     "Query total deposited amount across all accounts",
				},
				// this line is used by ignite scaffolding # autocli/query
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			// Use FQN string for Msg service as well
			Service:              "yuuchain.vault.v1.Msg",
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "UpdateParams",
					Skip:      true, // authority-gated
				},
				{
					RpcMethod:      "Deposit",
					Use:            "deposit [units]",
					Short:          "Send a deposit tx",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "units"}},
				},
				// this line is used by ignite scaffolding # autocli/tx
			},
		},
	}
}
