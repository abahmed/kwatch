package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
)

// oldProblemDays reaches the incident lifecycle in both editions.
func TestOldProblemDaysReachesTheLifecycle(t *testing.T) {
	cfg := &config.Config{OldProblemDays: 45}
	for _, e := range []feature.Entitlements{
		feature.ProEntitlements(), feature.CommunityEntitlements(),
	} {
		assert.Equal(t, 45*24*time.Hour,
			editionBuild(t, e, cfg).incidentConfig().OldProblem)
	}
	assert.Zero(t, editionBuild(t, feature.ProEntitlements(),
		&config.Config{}).incidentConfig().OldProblem)
}
