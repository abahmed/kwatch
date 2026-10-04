package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A DaemonSet below its desired count names the nodes whose pod is not
// ready, and the taints on them.
func TestDaemonSetGapsNameNodesAndTaints(t *testing.T) {
	m := newTestModel()
	ds := newID(kube.KindDaemonSet, "kube-system", "cni")
	put(m, ds, t0, nil)
	for _, c := range []struct {
		node   string
		ready  bool
		taints string
	}{{"n1", true, ""}, {"n2", false, "gpu=true:NoSchedule"},
		{"n3", false, ""}} {
		node := newID(kube.KindNode, "", c.node)
		attrs := map[string]inventory.Value{}
		if c.taints != "" {
			attrs[kube.AttrTaints] = inventory.Text(c.taints)
		}
		put(m, node, t0, attrs)
		pod := newID(kube.KindPod, "kube-system", "cni-"+c.node)
		put(m, pod, t0, map[string]inventory.Value{
			kube.AttrReady: inventory.Bool(c.ready)})
		relateEntity(m, pod, inventory.OwnedBy, ds)
		relateEntity(m, pod, inventory.RunsOn, node)
	}

	got := daemonSetGaps(m, entityOf(m, ds))

	assert.Equal(t, []detection.Evidence{
		{Label: "nodes without a ready pod", Value: "n2, n3"},
		{Label: "tainted nodes", Value: "n2 (gpu=true:NoSchedule)"},
	}, got)
}

func TestDaemonSetGapsOnlyForDaemonSets(t *testing.T) {
	m := newTestModel()
	deploy := newID(kube.KindDeployment, "shop", "api")
	put(m, deploy, t0, nil)

	assert.Nil(t, daemonSetGaps(m, entityOf(m, deploy)))
	assert.Nil(t, daemonSetGaps(nil, inventory.Entity{ID: newID(
		kube.KindDaemonSet, "kube-system", "cni")}))
}
