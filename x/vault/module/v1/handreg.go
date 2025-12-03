package v1

import (
    appconfig "cosmossdk.io/depinject/appconfig"
)

// This file only tells depinject that the proto module type exists.
// It does NOT provide the module. The actual provider is in app/app.go.
func init() {
    appconfig.RegisterModule(&Module{})
}
