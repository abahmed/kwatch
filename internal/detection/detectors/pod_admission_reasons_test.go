package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/detection/upstream"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// podReasonsThatAreNotAdmission are the upstream pod status reasons that
// a failed pod carries for another cause than a kubelet rejection:
// eviction, preemption, a lost or shut-down node, and the deadline.
var podReasonsThatAreNotAdmission = map[string]bool{
	"DeadlineExceeded": true, "Evicted": true, "NodeLost": true,
	"NodeShutdown": true, "Preempting": true, "Terminated": true,
}

// Every status reason the kubelet gives a pod it refused at admission
// is named as such, so a new Kubernetes reason is found by this test.
func TestPodAdmissionNamesEveryUpstreamRejection(t *testing.T) {
	for _, reason := range upstream.PodReasons {
		if podReasonsThatAreNotAdmission[reason] {
			continue
		}
		found := detectFailedPod(t, map[string]inventory.Value{
			kube.AttrReason: inventory.Text(reason),
		})
		require.Len(t, found, 1, reason)
		assert.Equal(t, reasons.PodAdmissionRejected, found[0].Reason,
			reason)
		assert.Equal(t, "Admission.Rejected."+reason,
			string(found[0].Mode))
	}
}

func TestPodAdmissionNamesRejectionsOutsideThePodReasonSnapshot(
	t *testing.T,
) {
	found := detectFailedPod(t, map[string]inventory.Value{
		kube.AttrReason: inventory.Text("VolumeAttachmentLimitExceeded"),
	})

	require.Len(t, found, 1)
	assert.Equal(t, reasons.PodAdmissionRejected, found[0].Reason)
}

// The kubelet refuses and ends pods of a node that is shutting down:
// the node going away is the story, not a rejection of the pod.
func TestPodNodeShutdownIsNotAnAdmissionFailure(t *testing.T) {
	found := detectFailedPod(t, map[string]inventory.Value{
		kube.AttrReason: inventory.Text("NodeShutdown"),
	})

	assert.Empty(t, found)
}
