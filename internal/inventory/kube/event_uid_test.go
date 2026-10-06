package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The kubelet names a Node by its name in involvedObject.uid, while the
// Node entity carries the real UID.
func TestKubeletNodeEventIsKeptAsEvidence(t *testing.T) {
	now := fixedTime()
	n := node("n1")
	n.UID = types.UID("real-node-uid")
	m := inventory.NewModel(inventory.Options{})
	applyAll(t, m, kube.NewTranslator(kube.NodeSchema{}).
		Added(n, true, now))

	ev := warningEvent("Node", "")
	ev.Reason = "SystemOOM"
	ev.InvolvedObject.Namespace = ""
	ev.InvolvedObject.Name = "n1"
	ev.InvolvedObject.UID = types.UID("n1")
	obs, ok := kube.EventNote(ev, now)
	require.True(t, ok)
	applyAll(t, m, []inventory.Observation{obs})

	id := inventory.CoreID(kube.KindNode, "", "n1")
	assert.Len(t, m.Notes(id, now.Add(-1)), 1)
}

func TestMirrorPodEventWithConfigHashIsKept(t *testing.T) {
	now := fixedTime()
	p := pod("etcd-n1")
	p.UID = types.UID("mirror-uid")
	p.Annotations = map[string]string{
		corev1.MirrorPodAnnotationKey: "config-hash",
	}
	m := inventory.NewModel(inventory.Options{})
	applyAll(t, m, kube.NewTranslator(kube.PodSchema{}).
		Added(p, true, now))

	ev := warningEvent("Pod", "")
	ev.InvolvedObject.Name = "etcd-n1"
	ev.InvolvedObject.UID = types.UID("config-hash")
	obs, ok := kube.EventNote(ev, now)
	require.True(t, ok)
	applyAll(t, m, []inventory.Observation{obs})

	id := inventory.CoreID(kube.KindPod, testNamespace, "etcd-n1")
	assert.Len(t, m.Notes(id, now.Add(-1)), 1)
}

func TestEventNoteCountsAnEventWithoutCountAsOne(t *testing.T) {
	ev := warningEvent("Pod", "")
	ev.Count = 0
	obs, ok := kube.EventNote(ev, fixedTime())
	require.True(t, ok)
	assert.Equal(t, 1, obs.Note.Count)
}
