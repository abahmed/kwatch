package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A new cluster has nodes that joined together, which is a wake-up. A pod
// created inside it that keeps failing its readiness probe is reported
// when the wake-up ends, not before: the end-to-end suite runs on such a
// cluster and must wait for it.
func TestReadinessFailureWaitsForNodesJoiningTogether(t *testing.T) {
	model := newTestModel()
	joined := podNodeNow.Add(-time.Minute)
	for _, name := range []string{"cp", "w1", "w2", "w3"} {
		observeEntity(model, inventory.CoreID(kube.KindNode, "", name),
			joined, map[string]inventory.Value{
				kube.AttrCreated: inventory.Time(joined)})
	}
	pod := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "app"}
	observeEntity(model, pod, joined, map[string]inventory.Value{
		kube.AttrPhase:   inventory.Text("Running"),
		kube.AttrCreated: inventory.Time(joined.Add(30 * time.Second)),
	})
	id := kube.ContainerID("default", "app", "main")
	observeEntity(model, id, joined, map[string]inventory.Value{
		kube.AttrState: inventory.Text("running"),
		kube.AttrReady: inventory.Bool(false),
		kube.AttrStartedAt: inventory.Time(
			joined.Add(40 * time.Second)),
	})
	relateEntity(model, id, inventory.PartOf, pod)
	noteEntity(model, id, "Unhealthy",
		"Readiness probe failed: HTTP probe failed with statuscode: 404",
		5, podNodeNow)

	during := Container{}.Detect(
		testDetectorContext(model, podNodeNow), entityOf(model, id))
	assert.Empty(t, during, "held while the cluster wakes up")

	end := joined.Add(kube.WakeQuiet)
	after := classified(Container{}.Detect(
		testDetectorContext(model, end), entityOf(model, id)))
	require.Len(t, after, 1, "reported once the wake-up is over")
	assert.Equal(t, reasons.ReadinessProbeFailed, after[0].Reason)
}
