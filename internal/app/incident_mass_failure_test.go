package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

func TestMassFailureUsesSharedDependencyIdentity(t *testing.T) {
	holder := &engineHolder{
		engine: incident.NewEngineWithClock(
			incident.Config{}, clock.RealClock{},
		),
	}
	dependency := "node//worker-a"
	notifyNewMassFailures(holder, map[string]insight.MassFailure{
		dependency: {
			SharedDependency: dependency,
			AffectedCount:    3,
			Reason:           "ServiceNoEndpoints",
			ResourceKind:     "service",
		},
	})
	tracked := holder.engine.MassFailureSet()
	created := tracked[incident.MassFailureKey(dependency)]
	require.NotNil(t, created)
	assert.Equal(t, constant.ReasonSharedDependencyFailure,
		created.Reason)
	assert.Equal(t, model.NewObjectRef("node", "", "worker-a"),
		created.Ref())
	assert.NotContains(t, created.Hint, "ServiceNoEndpoints")
}

func TestMassFailureRequiresDependencyEvidence(t *testing.T) {
	mf := insight.MassFailure{
		SharedDependency: "configmap/apps/settings",
		AffectedCount:    3,
	}
	_, supported := supportedMassFailure(mf, nil, nil)
	assert.False(t, supported)
	dependency := &model.Incident{
		Subject: model.Subject{
			Object: model.NewObjectRef(
				"configmap", "apps", "settings",
			),
		},
		Status: model.Status{State: model.StateActive},
	}
	_, supported = supportedMassFailure(
		mf, []*model.Incident{dependency}, nil,
	)
	assert.True(t, supported)
}

func TestMassFailureRequiresSustainedEvidence(t *testing.T) {
	seen := make(map[string]time.Time)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	key := "configmap/apps/settings"
	assert.False(t, massFailureSustained(seen, key, now, false))
	assert.False(t, massFailureSustained(
		seen, key, now.Add(massFailureSustain-time.Second), false,
	))
	assert.True(t, massFailureSustained(
		seen, key, now.Add(massFailureSustain), false,
	))
	delete(seen, key)
	assert.False(t, massFailureSustained(
		seen, key, now.Add(3*massFailureSustain), false,
	))
	assert.True(t, massFailureSustained(seen, key, now, true))
}
