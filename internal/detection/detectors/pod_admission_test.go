package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func detectFailedPod(
	t *testing.T, attrs map[string]inventory.Value,
) []detection.Finding {
	t.Helper()
	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "app"}
	attrs[kube.AttrPhase] = inventory.Text("Failed")
	observeEntity(model, id, podNodeNow, attrs)
	return classified(NewPod(PodThresholds{}).Detect(
		testDetectorContext(model, podNodeNow), entityOf(model, id)))
}

func TestPodAdmissionRejected(t *testing.T) {
	for _, reason := range []string{
		"OutOfcpu", "OutOfmemory", "OutOfpods", "OutOfephemeral-storage",
		"OutOfnvidia.com/gpu", "NodeAffinity", "UnexpectedAdmissionError",
		"InvalidNodeInfo", "PodOSNotSupported", "SysctlForbidden",
		"TopologyAffinityError",
	} {
		found := detectFailedPod(t, map[string]inventory.Value{
			kube.AttrReason:  inventory.Text(reason),
			kube.AttrMessage: inventory.Text("Node didn't have enough"),
		})
		require.Len(t, found, 1, reason)
		assert.Equal(t, reasons.PodAdmissionRejected, found[0].Reason)
		assert.Equal(t, "Admission.Rejected."+reason, string(found[0].Mode))
		assert.Equal(t, detection.Failing, found[0].Health)
		assert.Contains(t, found[0].Summary, reason)
	}
}

func TestPodAdmissionOtherFailuresStayGeneric(t *testing.T) {
	cases := map[string]string{
		"":                 reasons.PodFailed,
		"DeadlineExceeded": reasons.PodFailed,
		"OutOf":            reasons.PodFailed,
		reasons.Evicted:    reasons.Evicted,
	}
	for reason, want := range cases {
		attrs := map[string]inventory.Value{}
		if reason != "" {
			attrs[kube.AttrReason] = inventory.Text(reason)
		}
		found := detectFailedPod(t, attrs)
		require.Len(t, found, 1, reason)
		assert.Equal(t, want, found[0].Reason, reason)
	}
}
