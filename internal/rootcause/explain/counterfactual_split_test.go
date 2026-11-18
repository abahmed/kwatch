package explain

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestReplicaSplitZone(t *testing.T) {
	f := zoneFixture(t, false)
	pods := f.workloadPods("shop", "api", 4)

	got := ReplicaSplit(f.model, pods[:2], pods[2:])

	assert.Equal(t, []SplitDifference{{Attribute: SplitZone,
		Failing: "zone-b", Healthy: "zone-a"}}, got)
}

func TestReplicaSplitDigestLeads(t *testing.T) {
	f := digestFixture(t, digestA)
	pods := f.workloadPods("shop", "api", 4)

	got := ReplicaSplit(f.model, pods[:2], pods[2:])

	assert.Equal(t, []SplitDifference{{Attribute: SplitDigest,
		Image: imgRef, Failing: "sha256:9f1c0a2b44d1",
		Healthy: "sha256:aaaa1111bbbb"}}, got)
}

func TestReplicaSplitKeepsAtMostTwo(t *testing.T) {
	f := zoneFixture(t, false)
	pods := f.workloadPods("shop", "api", 4)
	for i, pod := range pods {
		qos := "Guaranteed"
		if i < 2 {
			qos = "BestEffort"
		}
		f.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: pod,
			Attributes: map[string]inventory.Value{
				kube.AttrQoS: inventory.Text(qos)}})
	}
	for i, pod := range pods {
		f.runContainer(containerOf(pod), imgRef,
			[]string{digestB, digestB, digestA, digestA}[i], t0)
	}

	got := ReplicaSplit(f.model, pods[:2], pods[2:])

	assert.Len(t, got, SplitMaxDifferences)
	assert.Equal(t, SplitDigest, got[0].Attribute)
	assert.Equal(t, SplitZone, got[1].Attribute,
		"the QoS class splits too but comes later")
}

func TestReplicaSplitSilent(t *testing.T) {
	mixed := zoneFixture(t, true)
	pods := mixed.workloadPods("shop", "api", 4)
	assert.Empty(t, ReplicaSplit(mixed.model, pods[:2], pods[2:]),
		"a healthy pod in the failing zone: no clean split")

	f := zoneFixture(t, false)
	pods = f.workloadPods("shop", "api", 4)
	assert.Empty(t, ReplicaSplit(f.model, pods[:1], pods[2:]),
		"one failing pod")
	assert.Empty(t, ReplicaSplit(f.model, pods, nil), "no healthy pod")
	assert.Empty(t, ReplicaSplit(nil, pods[:2], pods[2:]))
}

// workloadPods are the pod IDs fixture.workload made, in order.
func (f *fixture) workloadPods(
	namespace, name string, n int,
) []inventory.EntityID {
	out := make([]inventory.EntityID, 0, n)
	for i := range n {
		out = append(out, inventory.CoreID(kube.KindPod, namespace,
			name+"-1-"+string(rune('0'+i))))
	}
	return out
}
