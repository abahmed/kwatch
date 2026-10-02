package scope

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var maintenanceNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func maintenanceScope(t *testing.T, enabled bool) *Scope {
	t.Helper()
	cfg := config.Config{}
	scope, err := New(config.RuntimeConfigFor(&cfg).Scope(), nil,
		enabled, func() time.Time { return maintenanceNow })
	require.NoError(t, err)
	return scope
}

func observe(
	m *inventory.Model, id inventory.EntityID,
	attrs map[string]inventory.Value,
) {
	_, _ = m.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: maintenanceNow,
		Entity: id, Attributes: attrs,
	})
}

func held(value string) map[string]inventory.Value {
	return map[string]inventory.Value{
		kube.AttrMaintenance: inventory.Text(value)}
}

func until(at time.Time) map[string]inventory.Value {
	return map[string]inventory.Value{
		kube.AttrMaintenanceUntil: inventory.Time(at)}
}

func both(value string, at time.Time) map[string]inventory.Value {
	attrs := held(value)
	attrs[kube.AttrMaintenanceUntil] = inventory.Time(at)
	return attrs
}

func TestScopeMaintenanceHolds(t *testing.T) {
	podID := inventory.CoreID(kube.KindPod, "app", "web")
	nsID := inventory.CoreID(kube.KindNamespace, "", "app")
	cases := []struct {
		name    string
		enabled bool
		id      inventory.EntityID
		attrs   map[string]inventory.Value
		want    bool
	}{
		{"entity_on", true, podID, held("true"), false},
		{"entity_false_value", true, podID, held("false"), true},
		{"namespace_on", true, nsID, held("true"), false},
		{"until_future", true, podID,
			until(maintenanceNow.Add(time.Hour)), false},
		{"until_past", true, podID,
			until(maintenanceNow.Add(-time.Hour)), true},
		{"entity_off", true, podID, held("off"), true},
		{"entity_zero", true, podID, held("0"), true},
		{"entity_no", true, podID, held("no"), true},
		{"entity_yes_mixed_case", true, podID, held(" Yes "), false},
		{"entity_one", true, podID, held("1"), false},
		{"entity_on_value", true, podID, held("ON"), false},
		{"entity_garbage", true, podID, held("maybe"), true},
		{"both_until_future", true, podID,
			both("true", maintenanceNow.Add(time.Hour)), false},
		{"both_until_past", true, podID,
			both("true", maintenanceNow.Add(-time.Hour)), true},
		{"false_overrides_until", true, podID,
			both("false", maintenanceNow.Add(time.Hour)), true},
		{"disabled", false, podID, held("true"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := inventory.NewModel(inventory.Options{})
			observe(model, tc.id, tc.attrs)
			sig := containerFinding("app", "web", "c", "CrashLoopBackOff")
			sig.Entity = podID
			got := maintenanceScope(t, tc.enabled).Allows(model, sig)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestScopeMaintenanceOwningPodBlocksContainer(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	observe(model, inventory.CoreID(kube.KindPod, "app", "web"),
		held("true"))
	sig := containerFinding("app", "web", "c", "CrashLoopBackOff")
	assert.False(t, maintenanceScope(t, true).Allows(model, sig))
	other := containerFinding("app", "api", "c", "CrashLoopBackOff")
	assert.True(t, maintenanceScope(t, true).Allows(model, other))
}
