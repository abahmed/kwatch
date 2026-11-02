package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/scope"
)

// fakeTypedCoverage is the typed source as coverage reads it.
type fakeTypedCoverage struct{ synced time.Time }

func (fakeTypedCoverage) WatchedKinds() []kube.WatchedKind {
	return []kube.WatchedKind{{Kind: "pods", Mode: "full", Synced: true}}
}

func (fakeTypedCoverage) Unavailable() []kube.SourceStatus {
	return []kube.SourceStatus{{Resource: "secrets",
		Reason: kube.ReasonPermissionDenied}}
}

func (fakeTypedCoverage) Disabled() []kube.SourceStatus { return nil }

func (f fakeTypedCoverage) SyncedAt() time.Time { return f.synced }

// fakeDynamicCoverage is the dynamic source as coverage reads it.
type fakeDynamicCoverage struct{}

func (fakeDynamicCoverage) Status() kube.DynamicStatus {
	return kube.DynamicStatus{
		Watched: map[kube.WatchMode]int{kube.WatchStatus: 2}, Synced: 2,
		Complete: true,
	}
}

func TestDashboardCoverageReadsSourcesModelAndScope(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	for _, ns := range []string{"apps", "kube-system"} {
		_, err := model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: kube.ObservationSource,
			At: usageNow, Entity: inventory.CoreID(kube.KindNamespace, "", ns),
		})
		require.NoError(t, err)
	}
	cfg := config.Config{ForbiddenNamespaces: []string{"kube-system"}}
	findingScope, err := scope.New(config.RuntimeConfigFor(&cfg).Scope(),
		nil, false, nil)
	require.NoError(t, err)
	c := coverageSources{
		typed: fakeTypedCoverage{synced: usageNow}, model: model,
		dynamic: fakeDynamicCoverage{}, scope: findingScope,
	}

	got := c.build()

	assert.Equal(t, []string{"apps"}, got.Namespaces.WatchedNames)
	assert.Equal(t, []string{"kube-system"}, got.Namespaces.ExcludedNames)
	assert.Equal(t, 3, got.Kinds.Watched)
	assert.Equal(t, 3, got.Kinds.Synced)
	require.Len(t, got.Kinds.Unreadable, 1)
	assert.Equal(t, "secrets", got.Kinds.Unreadable[0].Kind)
	assert.Equal(t, usageNow, got.SyncedAt)
	assert.True(t, got.DiscoveryComplete)
}

func TestDashboardCoverageWithoutDynamicDiscoveryIsNotPartial(t *testing.T) {
	findingScope, err := scope.New(
		config.RuntimeConfigFor(&config.Config{}).Scope(), nil, false, nil)
	require.NoError(t, err)
	c := coverageSources{
		typed: fakeTypedCoverage{synced: usageNow},
		model: inventory.NewModel(inventory.Options{}), scope: findingScope,
	}

	assert.True(t, c.build().DiscoveryComplete)
}
