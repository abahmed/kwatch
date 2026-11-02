package incident

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// selectedWarehouse puts the pod a Service selects, owned by its
// Deployment, into the model, with no endpoint slice: the pod is not
// ready, and the slice is not (yet) known.
func selectedWarehouse(r *rig, svc inventory.EntityID) (
	deploy, pod inventory.EntityID,
) {
	deploy = entity(kube.KindDeployment, "warehouse")
	pod = entity(kube.KindPod, "warehouse-1")
	r.setAttrs(svc, map[string]inventory.Value{
		kube.AttrSelector: inventory.Text("app=warehouse"),
	})
	r.setAttrs(pod, map[string]inventory.Value{
		kube.AttrLabels: inventory.Text("app=warehouse"),
		kube.AttrReady:  inventory.Bool(false),
	})
	r.relate(pod, inventory.OwnedBy, deploy)
	return deploy, pod
}

// A restored Service incident whose pods still fail, now under the
// incident of their Deployment, is handed over to it when the grace
// ends. It must never resolve as healthy while they fail, even when no
// endpoint slice ties the Service to its pods: the selector does.
func TestRestoredServiceIncidentIsHandedOverThroughItsSelector(
	t *testing.T,
) {
	grace := 10 * time.Minute
	r, svc, _ := restoredServiceRig(t, grace)
	oldID := r.only().ID
	deploy, pod := selectedWarehouse(r, svc)
	r.raise(at(time.Minute),
		sig(pod, reasons.ReadinessProbeFailed, detection.Warning))

	var got []Decision
	for now := time.Minute; now <= grace+10*time.Minute; now += 10 *
		time.Second {
		got = append(got, r.tick(at(now))...)
	}
	var resolve *Decision
	for i, d := range got {
		if d.Action == Resolve && d.Incident.ID == oldID {
			resolve = &got[i]
		}
	}
	require.NotNil(t, resolve, "the Service incident is closed: %+v", got)
	assert.Equal(t, ReasonSuperseded, resolve.Reason)
	assert.False(t, strings.HasPrefix(string(resolve.Reason), "healthy"))
	assert.Equal(t, r.idOf(deploy), resolve.Incident.SupersededBy)
}

// A Service with no selector names no pods: the fallback never ties it
// to every pod of the namespace.
func TestWorkloadsOfServiceWithoutSelectorIsEmpty(t *testing.T) {
	r := newRig(t, Config{})
	svc := entity(kube.KindService, "external")
	r.setAttrs(svc, map[string]inventory.Value{
		kube.AttrSelector: inventory.Text("<none>"),
	})
	pod := entity(kube.KindPod, "web-1")
	r.setAttrs(pod, map[string]inventory.Value{
		kube.AttrLabels: inventory.Text("app=web"),
	})
	assert.Empty(t, workloadsOf(r.model, svc))
}

// With no incident holding them, failing pods that a restored Service's
// selector names still keep it from resolving as healthy: its workload
// is broken, so the resolve says tracking stopped, never "healthy".
func TestRestoredServiceIncidentWithBrokenSelectedPodsIsNotHealthy(
	t *testing.T,
) {
	grace := 10 * time.Minute
	r, svc, _ := restoredServiceRig(t, grace)
	oldID := r.only().ID
	_, pod := r.workloadRig(t, 1, 0, 4)
	r.setAttrs(svc, map[string]inventory.Value{
		kube.AttrSelector: inventory.Text("app=api"),
	})
	r.setAttrs(pod, map[string]inventory.Value{
		kube.AttrLabels: inventory.Text("app=api"),
		kube.AttrReady:  inventory.Bool(false),
	})
	r.raise(at(time.Minute)) // the manager reads this model from now on

	var got []Decision
	for now := time.Minute; now <= grace+StillBrokenMax+time.Hour; now +=
		time.Minute {
		got = append(got, r.tick(at(now))...)
	}
	for _, d := range got {
		if d.Action == Resolve && d.Incident.ID == oldID {
			assert.False(t,
				strings.HasPrefix(string(d.Reason), "healthy"), d.Reason)
			return
		}
	}
	t.Fatalf("the Service incident never closed: %+v", got)
}
