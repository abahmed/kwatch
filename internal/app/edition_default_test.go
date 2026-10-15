//go:build !licensetest

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/feature"
)

func TestDefaultBuildIgnoresTheEditionOverride(t *testing.T) {
	t.Setenv("KWATCH_EDITION_OVERRIDE", "pro")
	assert.Equal(t, feature.CommunityEdition,
		ResolveEntitlements().Edition())
}
