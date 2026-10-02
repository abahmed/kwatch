package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestEventClassifiesAttachWaitingForDetach(t *testing.T) {
	m := newTestModel()
	pod := newID(kube.KindPod, "ns", "db-0")
	put(m, pod, t0, nil)
	warn(m, pod, t0, "FailedAttachVolume",
		`Waiting for detach for volume "pvc-1" Volume is already `+
			"exclusively attached to one node")

	eval := evaluate(Event{}, m, t0, pod, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, reasons.VolumeAttachWaiting, eval.Findings[0].Reason)
	assert.Equal(t, "Volume.AttachWaiting", string(eval.Findings[0].Mode))
	assert.Contains(t, eval.Findings[0].Summary, "another node")
}

func TestEventKeepsOtherAttachFailures(t *testing.T) {
	m := newTestModel()
	pod := newID(kube.KindPod, "ns", "db-0")
	put(m, pod, t0, nil)
	warn(m, pod, t0, "FailedAttachVolume", "AttachVolume.Attach failed")

	eval := evaluate(Event{}, m, t0, pod, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "FailedAttachVolume", eval.Findings[0].Reason)
}

func TestEventMapsVolumeAndDeviceEvents(t *testing.T) {
	cases := map[string]string{
		"FailedMapVolume":               "Volume.MapFailed",
		"FailedPrepareDynamicResources": "Device.PrepareFailed",
		"ProvisioningFailed":            "Volume.ProvisioningFailed",
	}
	for reason, mode := range cases {
		m := newTestModel()
		id := newID(kube.KindPod, "ns", "p")
		put(m, id, t0, nil)
		warn(m, id, t0, reason, "failed")

		eval := evaluate(Event{}, m, t0, id, nil)

		require.Len(t, eval.Findings, 1, reason)
		assert.Equal(t, mode, string(eval.Findings[0].Mode), reason)
		assert.NotContains(t, eval.Findings[0].Summary, "reported", reason)
	}
}
