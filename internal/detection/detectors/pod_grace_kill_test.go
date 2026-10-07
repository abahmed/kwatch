package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// graceFixture is a pod of the api Deployment deleted 40 seconds ago with
// a 30 second grace period, whose container ended with exit finished
// after the grace period (finishedAfter seconds after the request).
type graceFixture struct {
	exit          float64
	finishedAfter time.Duration
	reason        string
	nodeReady     string
	hook          string
	updated       float64
}

func graceKill(t *testing.T, g graceFixture) []detection.Finding {
	t.Helper()
	model := newTestModel()
	node := inventory.CoreID(kube.KindNode, "", "n1")
	observeEntity(model, node, podNodeNow.Add(-time.Hour),
		conditionAttrs(nil, "Ready", g.nodeReady, "Test",
			podNodeNow.Add(-time.Hour)))
	deploy := inventory.CoreID(kube.KindDeployment, "shop", "api")
	observeEntity(model, deploy, podNodeNow.Add(-time.Hour),
		map[string]inventory.Value{
			kube.AttrReplicas:        inventory.Number(3),
			kube.AttrUpdatedReplicas: inventory.Number(g.updated),
		})
	requested := podNodeNow.Add(-40 * time.Second)
	pod := inventory.CoreID(kube.KindPod, "shop", "api-0")
	observeEntity(model, pod, podNodeNow.Add(-time.Hour),
		map[string]inventory.Value{
			kube.AttrPhase:    inventory.Text("Running"),
			kube.AttrDeleting: inventory.Bool(true),
			kube.AttrDeletionTime: inventory.Time(
				requested.Add(30 * time.Second)),
			kube.AttrTerminationGrace: inventory.Number(30),
		})
	relateEntity(model, pod, inventory.OwnedBy, deploy)
	relateEntity(model, pod, inventory.RunsOn, node)
	container := kube.ContainerID("shop", "api-0", "app")
	observeEntity(model, container, podNodeNow.Add(-time.Hour),
		map[string]inventory.Value{
			kube.AttrState:       inventory.Text("terminated"),
			kube.AttrStateReason: inventory.Text(g.reason),
			kube.AttrExitCode:    inventory.Number(g.exit),
			kube.AttrFinished: inventory.Time(
				requested.Add(g.finishedAfter)),
		})
	relateEntity(model, container, inventory.PartOf, pod)
	if g.hook != "" {
		noteEntity(model, pod, "FailedPreStopHook", g.hook, 1,
			requested.Add(5*time.Second))
	}
	return graceKillFindings(testDetectorContext(model, podNodeNow),
		entityOf(model, pod))
}

func okGrace() graceFixture {
	return graceFixture{exit: 137, finishedAfter: 31 * time.Second,
		reason: "Error", nodeReady: "True", updated: 1}
}

func TestPodKilledAtGraceEndsWithTheGracePeriod(t *testing.T) {
	found := graceKill(t, okGrace())

	require.Len(t, found, 1)
	assert.Equal(t, reasons.PodKilledAtGrace, found[0].Reason)
	assert.Equal(t, "30s",
		preemptEvidence(found[0], detection.EvidenceGracePeriod))
	assert.Equal(t, "app",
		preemptEvidence(found[0], detection.EvidenceKilledContainers))
	assert.Equal(t, "true",
		preemptEvidence(found[0], detection.EvidenceDuringRollout),
		"the new revision is not complete yet")
}

func TestPodKilledAtGraceOutsideARolloutSaysNothingOfOne(t *testing.T) {
	g := okGrace()
	g.updated = 3
	found := graceKill(t, g)

	require.Len(t, found, 1)
	assert.Empty(t,
		preemptEvidence(found[0], detection.EvidenceDuringRollout))
}

func TestPodKilledAtGraceQuotesAFailedPreStopHook(t *testing.T) {
	g := okGrace()
	g.hook = "Exec lifecycle hook ([sleep 60]) for Container \"app\" " +
		"in Pod \"api-0\" failed"
	found := graceKill(t, g)

	require.Len(t, found, 1)
	assert.Equal(t, g.hook,
		preemptEvidence(found[0], detection.EvidenceStopHook))
}

func TestPodStoppedBeforeTheGracePeriodIsNotAHardKill(t *testing.T) {
	g := okGrace()
	g.finishedAfter = 2 * time.Second
	assert.Empty(t, graceKill(t, g))
}

func TestOtherExitCodesAreNotHardKills(t *testing.T) {
	for _, exit := range []float64{0, 1, 143} {
		g := okGrace()
		g.exit = exit
		assert.Empty(t, graceKill(t, g), "exit %v", exit)
	}
	g := okGrace()
	g.reason = reasons.OOMKilled
	assert.Empty(t, graceKill(t, g), "an OOM kill is not a grace kill")
}

func TestPodsOfADeadNodeAreNotHardKills(t *testing.T) {
	for _, ready := range []string{"False", "Unknown"} {
		g := okGrace()
		g.nodeReady = ready
		assert.Empty(t, graceKill(t, g), "node Ready=%s", ready)
	}
}
