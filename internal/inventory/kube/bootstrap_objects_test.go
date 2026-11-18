package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Only the objects the control plane publishes wait, and only in the
// first moments of their namespace.
func TestBootstrapPendingOnlyInNewNamespaces(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	model := inventory.NewModel(inventory.Options{})
	_, _ = model.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: now,
		Entity: inventory.CoreID(KindNamespace, "", "fresh"),
		Attributes: map[string]inventory.Value{
			AttrCreated: inventory.Time(now.Add(-30 * time.Second))}})
	rootCA := inventory.CoreID(KindConfigMap, "fresh", RootCAConfigMap)
	account := inventory.CoreID(KindAccount, "fresh", DefaultServiceAccount)

	assert.Equal(t, 90*time.Second, BootstrapPending(model, rootCA, now))
	assert.Equal(t, 90*time.Second, BootstrapPending(model, account, now))
	assert.Zero(t, BootstrapPending(model,
		inventory.CoreID(KindConfigMap, "fresh", "settings"), now))
	assert.Zero(t, BootstrapPending(model, rootCA, now.Add(time.Hour)),
		"missing long after its namespace is a real finding")
	assert.Zero(t, BootstrapPending(model,
		inventory.CoreID(KindConfigMap, "old", RootCAConfigMap), now),
		"a namespace of unknown age gets no allowance")
}
