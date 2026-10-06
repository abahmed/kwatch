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

func versionedPod(name, uid, version string) *corev1.Pod {
	p := pod(name)
	p.UID = types.UID(uid)
	p.ResourceVersion = version
	return p
}

func applyAll(
	t *testing.T, m *inventory.Model, observations []inventory.Observation,
) {
	t.Helper()
	for _, o := range observations {
		_, err := m.Apply(o)
		require.NoError(t, err)
	}
}

// A pod re-created under the same name while the watch was down arrives as
// one update whose UID changed: it is a delete and a create.
func TestTranslatorTreatsAChangedUIDAsDeleteAndCreate(t *testing.T) {
	tr := kube.NewTranslator(kube.PodSchema{})
	old := versionedPod("pod-0", "uid-old", "10")
	fresh := versionedPod("pod-0", "uid-new", "20")

	observations := tr.Updated(old, fresh, fixedTime())

	id := inventory.CoreID(kube.KindPod, testNamespace, "pod-0")
	goneAt, observedAt := -1, -1
	for i, o := range observations {
		switch {
		case o.Kind == inventory.Gone && o.Entity == id:
			goneAt = i
		case o.Kind == inventory.Observed && o.Entity == id:
			observedAt = i
		}
	}
	require.GreaterOrEqual(t, goneAt, 0, "the old pod is gone")
	require.Greater(t, observedAt, goneAt, "then the new one appears")
}

func TestRecreatedStatefulSetPodDoesNotInheritFailedMount(t *testing.T) {
	tr := kube.NewTranslator(kube.PodSchema{})
	m := inventory.NewModel(inventory.Options{})
	old := versionedPod("pod-0", "uid-old", "10")
	applyAll(t, m, tr.Added(old, true, fixedTime()))

	event := warningEvent("Pod", "")
	event.Reason = "FailedMount"
	event.InvolvedObject.Name = "pod-0"
	event.InvolvedObject.Namespace = testNamespace
	event.InvolvedObject.UID = "uid-old"
	note, ok := kube.EventNote(event, fixedTime())
	require.True(t, ok)
	applyAll(t, m, []inventory.Observation{note})
	id := inventory.CoreID(kube.KindPod, testNamespace, "pod-0")
	require.Len(t, m.Notes(id, fixedTime().Add(-1)), 1)

	fresh := versionedPod("pod-0", "uid-new", "20")
	applyAll(t, m, tr.Updated(old, fresh, fixedTime()))

	assert.Empty(t, m.Notes(id, fixedTime().Add(-1)))
}

func TestTranslatorSkipsAResyncOfAnUnchangedVersion(t *testing.T) {
	tr := kube.NewTranslator(kube.PodSchema{})
	same := versionedPod("p", "uid", "10")
	assert.Empty(t, tr.Updated(same, versionedPod("p", "uid", "10"),
		fixedTime()))
}

func TestTranslatorKeepsUpdatesWithoutAVersion(t *testing.T) {
	tr := kube.NewTranslator(kube.PodSchema{})
	assert.NotEmpty(t, tr.Updated(pod("p"), pod("p"), fixedTime()))
}

func TestTranslatorKeepsAChangedVersion(t *testing.T) {
	tr := kube.NewTranslator(kube.PodSchema{})
	assert.NotEmpty(t, tr.Updated(versionedPod("p", "u", "10"),
		versionedPod("p", "u", "11"), fixedTime()))
}
