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

const multiAttachMessage = `Multi-Attach error for volume "pvc-1" Volume ` +
	"is already exclusively attached to one node and can't be attached " +
	"to another"

func TestEventClassifiesMultiAttachAndHoldsItBriefly(t *testing.T) {
	m := newTestModel()
	pod := newID(kube.KindPod, "ns", "api-7f")
	put(m, pod, t0, nil)
	warn(m, pod, t0, "FailedAttachVolume", multiAttachMessage)

	quiet := evaluate(Event{}, m, t0.Add(time.Minute), pod, nil)
	assert.Empty(t, quiet.Findings, "a rollout overlap clears by itself")

	eval := evaluate(Event{}, m, t0.Add(DefaultMultiAttachHold), pod, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, reasons.VolumeMultiAttach, eval.Findings[0].Reason)
	assert.Equal(t, "Volume.MultiAttach", string(eval.Findings[0].Mode))
	assert.Contains(t, eval.Findings[0].Summary, "another node")
}

func TestEventMultiAttachUsedByPodsVariant(t *testing.T) {
	m := newTestModel()
	pod := newID(kube.KindPod, "ns", "api-7f")
	put(m, pod, t0, nil)
	warn(m, pod, t0, "FailedAttachVolume", `Multi-Attach error for volume `+
		`"pvc-1" Volume is already used by pod(s) api-6d`)

	eval := evaluate(Event{}, m, t0.Add(DefaultMultiAttachHold), pod, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, reasons.VolumeMultiAttach, eval.Findings[0].Reason)
}

func TestEventMultiAttachNamesWhatHoldsTheVolume(t *testing.T) {
	m := newTestModel()
	claim := newID(kube.KindPVC, "ns", "data")
	n1, n2 := newID(kube.KindNode, "", "n1"), newID(kube.KindNode, "", "n2")
	oldPod := newID(kube.KindPod, "ns", "api-6d")
	pod := newID(kube.KindPod, "ns", "api-7f")
	put(m, claim, t0, nil)
	put(m, oldPod, t0, map[string]inventory.Value{
		kube.AttrDeleting: inventory.Bool(true)})
	put(m, pod, t0, nil)
	link(m, oldPod, inventory.Mounts, claim)
	link(m, pod, inventory.Mounts, claim)
	link(m, oldPod, inventory.RunsOn, n1)
	link(m, pod, inventory.RunsOn, n2)
	warn(m, pod, t0, "FailedAttachVolume", multiAttachMessage)

	eval := evaluate(Event{}, m, t0.Add(DefaultMultiAttachHold), pod, nil)

	require.Len(t, eval.Findings, 1)
	evidence := eval.Findings[0].Evidence
	assert.Contains(t, evidence, detection.Evidence{
		Label: detection.EvidenceHeldVolume, Value: "pvc-1"})
	assert.Contains(t, evidence, detection.Evidence{
		Label: detection.EvidenceAttachNode, Value: "n1"})
	assert.Contains(t, evidence, detection.Evidence{
		Label: detection.EvidenceHeldPod, Value: "api-6d"})
	assert.Contains(t, evidence, detection.Evidence{
		Label: detection.EvidenceHeldPodEnding, Value: "terminating"})
}
