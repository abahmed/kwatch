package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// goneModel holds the given objects, plus a pod and a deployment kept
// elsewhere so the model tracks both kinds.
func goneModel(t *testing.T, ids ...inventory.EntityID) *inventory.Model {
	t.Helper()
	m := inventory.NewModel(inventory.Options{})
	ids = append(ids,
		inventory.CoreID(kube.KindPod, "other", "keeper"),
		inventory.CoreID(kube.KindDeployment, "other", "keeper"))
	for _, id := range ids {
		_, err := m.Apply(inventory.Observation{Kind: inventory.Observed,
			Source: "test", At: roundAt, Entity: id})
		require.NoError(t, err)
	}
	return m
}

func podIncident(root inventory.EntityID, pods ...inventory.EntityID,
) incident.Incident {
	p := holderIncident(root, detection.ModeNoEndpoints, pods...)
	return p
}

// A step that describes a pod deleted from the cluster is dropped, one
// whose object exists is kept.
func TestStepNamingADeletedObjectIsDropped(t *testing.T) {
	live := inventory.CoreID(kube.KindPod, "shop", "live")
	dead := inventory.CoreID(kube.KindPod, "shop", "dead")
	root := inventory.CoreID(kube.KindDeployment, "shop", "api")
	m := goneModel(t, live, root)
	p := podIncident(root, live, dead)
	steps := []FixStep{
		{Text: "a", Command: "kubectl describe pod dead -n shop"},
		{Text: "b", Command: "kubectl describe pod live -n shop"}}

	got := currentSteps(steps, p, m)

	assert.Equal(t, steps[1:], got)
	assert.Equal(t, steps, currentSteps(steps, p, nil),
		"without a model nothing is judged")
}

// With every step dropped the root is described; when the root is gone
// too a line says it no longer exists.
func TestStepsFallBackToTheRootOrSayItIsGone(t *testing.T) {
	dead := inventory.CoreID(kube.KindPod, "shop", "dead")
	root := inventory.CoreID(kube.KindDeployment, "shop", "api")
	p := podIncident(root, dead)
	steps := []FixStep{{Text: "a",
		Command: "kubectl describe pod dead -n shop"}}

	got := currentSteps(steps, p, goneModel(t, root))
	require.Len(t, got, 1)
	assert.Equal(t, rootStepText, got[0].Text)
	assert.Equal(t, "kubectl -n shop describe deployment api",
		got[0].Command)

	got = currentSteps(steps, p, goneModel(t))
	assert.Equal(t, []FixStep{{Text: goneStepText}}, got)
}
