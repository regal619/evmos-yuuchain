package types

// DefaultParams returns minimal module params.
func DefaultParams() Params { return Params{} }

// DefaultGenesis returns a valid minimal genesis state.
func DefaultGenesis() *GenesisState {
p := DefaultParams()
return &GenesisState{
Params: p, // GenesisState expects *Params
}
}

// Validate performs basic validation of the genesis state.
func (gs GenesisState) Validate() error {
return gs.Params.Validate()
}

// Validate validates module parameters.
func (p *Params) Validate() error {
return nil
}
