package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// addContainer puts a container with attrs into pod.
func addContainer(
	t *testing.T, r *rig, pod inventory.EntityID, name string,
	attrs map[string]inventory.Value,
) {
	t.Helper()
	id := kube.ContainerID(pod.Namespace, pod.Name, name)
	observeAttrs(t, r, id, attrs)
	r.relate(id, inventory.PartOf, pod)
}

func crashing(restarts float64) map[string]inventory.Value {
	return map[string]inventory.Value{
		kube.AttrReady:        inventory.Bool(false),
		kube.AttrRestarts:     inventory.Number(restarts),
		kube.AttrStateReason:  inventory.Text("CrashLoopBackOff"),
		kube.AttrLastReason:   inventory.Text("OOMKilled"),
		kube.AttrLastExitCode: inventory.Number(137),
		kube.AttrImage:        inventory.Text("registry.io/shop/web:1.2"),
		kube.AttrMemoryLimit:  inventory.Number(256 << 20),
	}
}

func TestBreakSnapshotKeepsFirstAndWorst(t *testing.T) {
	r := newRig(t, Config{})
	pod := entity(kube.KindPod, "web")
	r.observe(pod)
	addContainer(t, r, pod, "app", crashing(3))
	addContainer(t, r, pod, "sidecar", map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(true)})
	r.raise(at(0), podSig("web"))

	breaks := r.only().Journal.Breaks
	require.Len(t, breaks, 1)
	first := breaks[0]
	assert.Equal(t, BreakFirst, first.Reason)
	assert.Equal(t, at(0), first.At)
	require.Len(t, first.Containers, 1, "only the failing container")
	row := first.Containers[0]
	assert.Equal(t, "app", row.Container)
	assert.Equal(t, 3, row.Restarts)
	require.NotNil(t, row.LastExitCode)
	assert.Equal(t, 137, *row.LastExitCode)
	assert.Equal(t, int64(256<<20), row.MemoryLimit)

	addContainer(t, r, pod, "app", crashing(9))
	r.m.mu.Lock()
	for i := 0; i < 3; i++ {
		r.m.takeBreak(r.m.lookup(pod), at(time.Duration(i+1)*time.Minute),
			BreakWorse)
	}
	r.m.mu.Unlock()
	breaks = r.only().Journal.Breaks
	require.Len(t, breaks, maxBreaks)
	assert.Equal(t, first.At, breaks[0].At, "the first break stays")
	assert.Equal(t, BreakWorse, breaks[1].Reason)
	assert.Equal(t, at(3*time.Minute), breaks[1].At)
	assert.Equal(t, 9, breaks[1].Containers[0].Restarts)
}

func TestBreakSnapshotSkipsIncidentsWithoutFailingContainers(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))
	assert.Empty(t, r.only().Journal.Breaks)
}

func TestBreakSnapshotIsBoundedAndPersisted(t *testing.T) {
	r := newRig(t, Config{})
	pod := entity(kube.KindPod, "web")
	r.observe(pod)
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h",
		"i", "j", "k", "l"} {
		addContainer(t, r, pod, name, crashing(1))
	}
	r.raise(at(0), podSig("web"))
	records := r.m.Export()
	require.Len(t, records[0].Breaks, 1)
	assert.Len(t, records[0].Breaks[0].Containers, maxBreakContainers)
	assert.Equal(t, "web:1.2",
		records[0].Breaks[0].Containers[0].Image[len("registry.io/shop/"):])
}

func TestContainersOfCutsToLimit(t *testing.T) {
	r := newRig(t, Config{})
	pod := entity(kube.KindPod, "web")
	r.observe(pod)
	addContainer(t, r, pod, "a", crashing(1))
	addContainer(t, r, pod, "b", crashing(5))
	rows, cut := ContainersOf(r.model, pod, nil, 1)
	require.Len(t, rows, 1)
	assert.Equal(t, "b", rows[0].Container, "most restarted first")
	assert.Equal(t, 1, cut)
}
