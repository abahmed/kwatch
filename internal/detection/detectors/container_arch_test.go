package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const execFormatLine = "exec /app/api: exec format error"

// archFixture is a crashing api pod on node n1 of arch, and healthy
// api pods on nodes of other architectures.
func archFixture(
	t *testing.T, arch string, message string, healthy ...string,
) detection.Finding {
	t.Helper()
	model := newTestModel()
	deploy := inventory.CoreID(kube.KindDeployment, "shop", "api")
	observeEntity(model, deploy, podNodeNow.Add(-time.Hour), nil)
	addArchNode(model, "n1", arch)
	victim := addArchPod(model, deploy, "api-0", "n1", false)
	observeEntity(model, victim, podNodeNow.Add(-time.Hour),
		map[string]inventory.Value{
			kube.AttrState:       inventory.Text("waiting"),
			kube.AttrStateReason: inventory.Text("CrashLoopBackOff"),
			kube.AttrImage:       inventory.Text("ghcr.io/x/api:1.4"),
			kube.AttrLastMessage: inventory.Text(message),
		})
	for i, other := range healthy {
		node := "h" + string(rune('0'+i))
		addArchNode(model, node, other)
		addArchPod(model, deploy, "api-"+node, node, true)
	}
	found := Container{}.Detect(testDetectorContext(model, podNodeNow),
		entityOf(model, victim))
	require.Len(t, found, 1)
	return found[0]
}

func addArchNode(model *inventory.Model, name, arch string) {
	observeEntity(model, inventory.CoreID(kube.KindNode, "", name),
		podNodeNow.Add(-time.Hour), map[string]inventory.Value{
			kube.AttrNodeLabels: inventory.Text(
				"kubernetes.io/arch=" + arch + ",kubernetes.io/os=linux"),
		})
}

// addArchPod adds a pod of deploy on node and returns its container.
func addArchPod(
	model *inventory.Model, deploy inventory.EntityID, name, node string,
	ready bool,
) inventory.EntityID {
	pod := inventory.CoreID(kube.KindPod, "shop", name)
	observeEntity(model, pod, podNodeNow.Add(-time.Hour),
		map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Running"),
			kube.AttrReady: inventory.Bool(ready),
		})
	relateEntity(model, pod, inventory.OwnedBy, deploy)
	relateEntity(model, pod, inventory.RunsOn,
		inventory.CoreID(kube.KindNode, "", node))
	id := kube.ContainerID("shop", name, "app")
	relateEntity(model, id, inventory.PartOf, pod)
	return id
}

func TestExecFormatErrorNamesTheNodeArchitecture(t *testing.T) {
	f := archFixture(t, "arm64", execFormatLine, "amd64", "amd64")
	assert.Contains(t, f.Summary, "arm64")
	assert.Contains(t, f.Summary, "exec format error")
	assert.Contains(t, f.Summary, "n1")
	ev := map[string]string{}
	for _, e := range f.Evidence {
		ev[e.Label] = e.Value
	}
	assert.Equal(t, "arm64 node n1", ev[detection.EvidenceArchNode])
	assert.Equal(t, "2 pods on amd64 nodes",
		ev[detection.EvidenceArchHealthy])
	assert.Equal(t, execFormatLine, ev[detection.EvidenceError])
}

func TestExecFormatErrorWithoutHealthyReplicas(t *testing.T) {
	f := archFixture(t, "arm64", execFormatLine)
	for _, e := range f.Evidence {
		assert.NotEqual(t, detection.EvidenceArchHealthy, e.Label)
	}
	assert.Contains(t, f.Summary, "arm64")
}

func TestOtherErrorsAreNotAnArchitectureProblem(t *testing.T) {
	f := archFixture(t, "arm64", "panic: nil pointer", "amd64")
	assert.NotContains(t, f.Summary, "arm64")
}

func TestExecFormatErrorNeedsTheNodeArchitecture(t *testing.T) {
	f := archFixture(t, "", execFormatLine)
	assert.NotContains(t, f.Summary, "architecture")
}
