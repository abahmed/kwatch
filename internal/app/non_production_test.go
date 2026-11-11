package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
)

// app.environment: non-production reaches the incident lifecycle, which
// then caps every tier below a page.
func TestNonProductionReachesTheLifecycle(t *testing.T) {
	cfg := &config.Config{App: config.App{Environment: "non-production"}}
	assert.True(t, editionBuild(t, feature.ProEntitlements(), cfg).
		incidentConfig().NonProduction)
	assert.False(t, editionBuild(t, feature.ProEntitlements(),
		&config.Config{}).incidentConfig().NonProduction)
}
