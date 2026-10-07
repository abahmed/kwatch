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

const mib = 1 << 20

// oomFixture is a container killed for memory on node n1, which also
// runs a big importer without a limit.
type oomFixture struct {
	peak     float64
	limit    float64
	pressure bool
	// event is the node event reason and its age before the kill.
	event    string
	message  string
	eventAge time.Duration
}

func detectOOM(t *testing.T, f oomFixture) detection.Finding {
	t.Helper()
	model := newTestModel()
	node := inventory.CoreID(kube.KindNode, "", "n1")
	nodeAttrs := map[string]inventory.Value{
		kube.AttrMemoryAllocatable: inventory.Number(16 * 1024 * mib),
	}
	status := "False"
	if f.pressure {
		status = "True"
	}
	conditionAttrs(nodeAttrs, "MemoryPressure", status, "Test",
		podNodeNow.Add(-time.Hour))
	observeEntity(model, node, podNodeNow.Add(-time.Hour), nodeAttrs)
	if f.event != "" {
		noteEntity(model, node, f.event, f.message, 1,
			podNodeNow.Add(-time.Minute-f.eventAge))
	}
	addPodOn(model, node, "batch", "importer", "main", 3174*mib, 0, nil)
	victim := addPodOn(model, node, "shop", "api", "app", f.peak, f.limit,
		map[string]inventory.Value{
			kube.AttrState:         inventory.Text("terminated"),
			kube.AttrStateReason:   inventory.Text(reasons.OOMKilled),
			kube.AttrExitCode:      inventory.Number(137),
			kube.AttrMemoryPeak24h: inventory.Number(f.peak),
			kube.AttrLastFinished: inventory.Time(
				podNodeNow.Add(-time.Minute)),
		})
	found := Container{}.Detect(testDetectorContext(model, podNodeNow),
		entityOf(model, victim))
	require.Len(t, found, 1)
	require.Equal(t, reasons.OOMKilled, found[0].Reason)
	return found[0]
}

// addPodOn adds a pod running on node with one container using bytes
// of memory, and returns the container.
func addPodOn(
	model *inventory.Model, node inventory.EntityID, ns, name, container string,
	bytes, limit float64, more map[string]inventory.Value,
) inventory.EntityID {
	pod := inventory.CoreID(kube.KindPod, ns, name)
	observeEntity(model, pod, podNodeNow.Add(-time.Hour),
		map[string]inventory.Value{kube.AttrPhase: inventory.Text("Running")})
	relateEntity(model, pod, inventory.RunsOn, node)
	id := kube.ContainerID(ns, name, container)
	attrs := map[string]inventory.Value{
		kube.AttrMemoryWorking: inventory.Number(bytes),
	}
	if limit > 0 {
		attrs[kube.AttrMemoryLimit] = inventory.Number(limit)
	}
	for name, value := range more {
		attrs[name] = value
	}
	observeEntity(model, id, podNodeNow.Add(-time.Hour), attrs)
	relateEntity(model, id, inventory.PartOf, pod)
	return id
}

func usersOf(f detection.Finding) []string {
	var out []string
	for _, e := range f.Evidence {
		if e.Label == detection.EvidenceNodeMemoryUser {
			out = append(out, e.Value)
		}
	}
	return out
}

func TestOOMDuringSystemOOMIsTheNodes(t *testing.T) {
	f := detectOOM(t, oomFixture{peak: 180 * mib, limit: 512 * mib,
		event: "SystemOOM", message: "System OOM encountered, victim " +
			"process: importer, pid: 9"})

	assert.Equal(t, "n1", evidenceOf(f, detection.EvidenceKilledByNode))
	assert.Equal(t, "180Mi", evidenceOf(f, detection.EvidenceMemoryUsed))
	assert.Equal(t, "512Mi", evidenceOf(f, detection.EvidenceMemoryLimit))
	assert.Contains(t, f.Summary, "by the node")
	assert.Equal(t, []string{"batch/importer 3.1Gi (no limit)"}, usersOf(f))
}

func TestOOMKilledByKernelEventIsTheNodes(t *testing.T) {
	f := detectOOM(t, oomFixture{peak: 180 * mib, limit: 512 * mib,
		event: "OOMKilling", message: "Out of memory: Killed process 9 " +
			"(importer) total-vm:1kB"})

	assert.Equal(t, "n1", evidenceOf(f, detection.EvidenceKilledByNode))
}

func TestOOMInsideItsOwnCgroupIsNotTheNodes(t *testing.T) {
	f := detectOOM(t, oomFixture{peak: 180 * mib, limit: 512 * mib,
		event: "OOMKilling", message: "Memory cgroup out of memory: " +
			"Killed process 9 (api)"})

	assert.Empty(t, evidenceOf(f, detection.EvidenceKilledByNode))
}

func TestOOMAtItsLimitIsNotTheNodesEvenDuringSystemOOM(t *testing.T) {
	f := detectOOM(t, oomFixture{peak: 500 * mib, limit: 512 * mib,
		event: "SystemOOM", message: "System OOM encountered"})

	assert.Empty(t, evidenceOf(f, detection.EvidenceKilledByNode))
	assert.NotContains(t, f.Summary, "by the node")
}

func TestOOMUnderItsLimitDuringMemoryPressureIsTheNodes(t *testing.T) {
	f := detectOOM(t, oomFixture{peak: 180 * mib, limit: 512 * mib,
		pressure: true})

	assert.Equal(t, "n1", evidenceOf(f, detection.EvidenceKilledByNode))
}

func TestOOMAtItsLimitDuringMemoryPressureIsNotTheNodes(t *testing.T) {
	f := detectOOM(t, oomFixture{peak: 500 * mib, limit: 512 * mib,
		pressure: true})

	assert.Empty(t, evidenceOf(f, detection.EvidenceKilledByNode))
}

func TestOOMWithoutNodeSignalIsNotTheNodes(t *testing.T) {
	f := detectOOM(t, oomFixture{peak: 180 * mib, limit: 512 * mib})

	assert.Empty(t, evidenceOf(f, detection.EvidenceKilledByNode))
}

func TestOOMLongAfterASystemOOMIsNotTheNodes(t *testing.T) {
	f := detectOOM(t, oomFixture{peak: 180 * mib, limit: 512 * mib,
		event: "SystemOOM", message: "System OOM encountered",
		eventAge: time.Hour})

	assert.Empty(t, evidenceOf(f, detection.EvidenceKilledByNode))
}

func TestOOMWithoutALimitDuringSystemOOMIsTheNodes(t *testing.T) {
	f := detectOOM(t, oomFixture{peak: 900 * mib,
		event: "SystemOOM", message: "System OOM encountered"})

	assert.Equal(t, "n1", evidenceOf(f, detection.EvidenceKilledByNode))
	assert.Empty(t, evidenceOf(f, detection.EvidenceMemoryLimit))
}
