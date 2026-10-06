package incident

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// setAttrs applies attributes to an entity of the rig's model.
func (r *rig) setAttrs(
	id inventory.EntityID, attrs map[string]inventory.Value,
) {
	r.t.Helper()
	_, err := r.model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: t0, Entity: id,
		Attributes: attrs,
	})
	require.NoError(r.t, err)
}

// workloadRig puts a Deployment with the given replica counts and one
// pod into the model. The pod is not ready and its container restarted.
func (r *rig) workloadRig(
	t *testing.T, desired, ready float64, restarts float64,
) (deploy, pod inventory.EntityID) {
	t.Helper()
	deploy = entity(kube.KindDeployment, "api")
	pod = entity(kube.KindPod, "api-1")
	container := entity(kube.KindContainer, "api-1.app")
	r.setAttrs(deploy, map[string]inventory.Value{
		kube.AttrReplicas:      inventory.Number(desired),
		kube.AttrReadyReplicas: inventory.Number(ready),
	})
	r.setAttrs(pod, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(false),
	})
	r.setAttrs(container, map[string]inventory.Value{
		kube.AttrRestarts: inventory.Number(restarts),
	})
	r.relate(pod, inventory.OwnedBy, deploy)
	r.relate(container, inventory.PartOf, pod)
	return deploy, pod
}

// crashSig is a Critical crash-loop finding of pod.
func crashSig(pod inventory.EntityID) detection.Finding {
	f := sig(pod, reasons.CrashLoopBackOff, detection.Critical)
	f.Mode = detection.ModeCrashLoop
	return f
}

// notReadySig is a Warning not-ready finding of pod.
func notReadySig(pod inventory.EntityID) detection.Finding {
	f := sig(pod, reasons.NotReady, detection.Warning)
	f.Mode = detection.ModeNotReady
	return f
}
