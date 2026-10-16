//go:build !licensetest

package app

import "github.com/abahmed/kwatch/internal/feature"

// editionOverride is the production edition: no override exists.
func editionOverride(e feature.Entitlements) feature.Entitlements { return e }
