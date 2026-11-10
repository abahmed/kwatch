package detectors

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A claim that binds on its first consumer waits by design while its
// pod waits for a node (an autoscaler adding one): no finding. Once the
// pod is scheduled, a claim still Pending is a real wait.
func TestClaimWaitsWhileItsConsumerIsUnscheduled(t *testing.T) {
	for name, scheduled := range map[string]bool{
		"pod waits for a node": false,
		"pod scheduled":        true,
	} {
		t.Run(name, func(t *testing.T) {
			m := newTestModel()
			pvc := newID(kube.KindPVC, "ns", "data")
			sc := newID(kube.KindStorageClass, "", "wffc")
			pod := newID(kube.KindPod, "ns", "p")
			put(m, sc, t0, map[string]inventory.Value{
				kube.AttrBindingMode: inventory.Text(waitForFirstConsumer)})
			put(m, pvc, t0, map[string]inventory.Value{
				kube.AttrPhase: inventory.Text("Pending")})
			podAttrs := map[string]inventory.Value{
				kube.AttrPhase: inventory.Text("Pending")}
			if scheduled {
				podAttrs[kube.ConditionKey("PodScheduled")] =
					inventory.Text("True")
			}
			put(m, pod, t0, podAttrs)
			link(m, pvc, inventory.References, sc)
			link(m, pod, inventory.Mounts, pvc)
			got := evaluate(Claim{}, m, t0.Add(10*time.Minute), pvc, nil)
			waiting := false
			for _, f := range got.Findings {
				waiting = waiting ||
					f.Reason == reasons.PersistentVolumeClaim
			}
			if waiting != scheduled {
				t.Fatalf("claim finding = %v, want %v", waiting, scheduled)
			}
		})
	}
}
