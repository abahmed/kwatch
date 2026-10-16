//go:build licensetest

package app

import (
	"os"

	"github.com/abahmed/kwatch/internal/feature"
)

// editionOverride lets a test build choose the Pro edition with
// KWATCH_EDITION_OVERRIDE=pro. Only binaries built with the licensetest
// tag read it.
func editionOverride(e feature.Entitlements) feature.Entitlements {
	if os.Getenv("KWATCH_EDITION_OVERRIDE") == "pro" {
		return feature.ProEntitlements()
	}
	return e
}
