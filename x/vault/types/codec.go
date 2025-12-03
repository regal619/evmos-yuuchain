// x/vault/types/codec.go
package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
        "github.com/cosmos/cosmos-sdk/types/msgservice"
)

// If you add Amino types later, wire them here. For now it's empty.
func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {}

// RegisterInterfaces registers the vault messages with the interface registry.
// We intentionally DO NOT call msgservice.RegisterMsgServiceDesc here.
// The Msg service is wired in AppModule.RegisterServices.
func RegisterInterfaces(reg codectypes.InterfaceRegistry) {
	reg.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgUpdateParams{},
		&MsgDeposit{},
                &MsgMintFromBridge{},
	)

     msgservice.RegisterMsgServiceDesc(reg, &_Msg_serviceDesc)
}
