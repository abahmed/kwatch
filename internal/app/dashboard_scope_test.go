package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The dashboard's scope carries the namespace label selector: a
// namespace whose labels the selector refuses is out, though no list
// names it.
func TestDashboardScopeHonoursTheNamespaceSelector(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	for name, labels := range map[string]string{
		"shop": "alerts=on", "dev": "alerts=off",
	} {
		_, err := model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: kube.ObservationSource,
			At:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Entity: inventory.CoreID(kube.KindNamespace, "", name),
			Attributes: map[string]inventory.Value{
				kube.AttrLabels: inventory.Text(labels)},
		})
		require.NoError(t, err)
	}
	cfg := config.Config{NamespaceSelector: "alerts=on"}

	got := dashboardScope(
		config.RuntimeConfigFor(&cfg).Scope(), clock.RealClock{})

	require.NotNil(t, got.Allowed)
	assert.True(t, got.Allowed(model, "shop"))
	assert.False(t, got.Allowed(model, "dev"))
	assert.Empty(t, got.Watched)
	assert.Empty(t, got.Excluded)
}

func TestDashboardScopeKeepsTheNamespaceLists(t *testing.T) {
	cfg := config.Config{AllowedNamespaces: []string{"shop"},
		ForbiddenNamespaces: []string{"dev"}}

	got := dashboardScope(
		config.RuntimeConfigFor(&cfg).Scope(), clock.RealClock{})

	assert.Equal(t, []string{"shop"}, got.Watched)
	assert.Equal(t, []string{"dev"}, got.Excluded)
	model := inventory.NewModel(inventory.Options{})
	assert.True(t, got.Allowed(model, "shop"))
	assert.False(t, got.Allowed(model, "dev"))
}
