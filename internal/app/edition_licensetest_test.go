//go:build licensetest

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/feature"
)

func TestLicenseTestBuildHonoursTheEditionOverride(t *testing.T) {
	t.Setenv("KWATCH_EDITION_OVERRIDE", "pro")
	assert.Equal(t, feature.ProEdition, ResolveEntitlements().Edition())
	t.Setenv("KWATCH_EDITION_OVERRIDE", "")
	assert.Equal(t, feature.CommunityEdition,
		ResolveEntitlements().Edition())
	t.Setenv("KWATCH_EDITION_OVERRIDE", "community")
	assert.Equal(t, feature.CommunityEdition,
		ResolveEntitlements().Edition())
}
