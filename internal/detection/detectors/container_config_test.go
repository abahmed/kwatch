package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// configErrorFixture is a pod whose container waits with
// CreateContainerConfigError. When secret is not empty the pod
// references that Secret, which does not exist.
func configErrorFixture(
	t *testing.T, secret string,
) (*inventory.Model, inventory.Entity) {
	t.Helper()
	m := newTestModel()
	pod := newID(kube.KindPod, "default", "web")
	put(m, pod, t0, nil)
	ctr := newID(kube.KindContainer, "default", "web/app")
	put(m, ctr, t0, map[string]inventory.Value{
		kube.AttrState:       inventory.Text("waiting"),
		kube.AttrStateReason: inventory.Text(reasons.CreateConfigError),
		kube.AttrMessage:     inventory.Text("kubelet said no"),
	})
	link(m, ctr, inventory.PartOf, pod)
	if secret != "" {
		link(m, pod, inventory.References,
			newID(kube.KindSecret, "default", secret))
	}
	e, ok := m.Entity(ctr)
	require.True(t, ok)
	return m, e
}

// The "references something missing" wording is for a container whose
// pod really references a missing object.
func TestConfigErrorNamesMissingReference(t *testing.T) {
	m, e := configErrorFixture(t, "creds")

	got := Container{}.Detect(testDetectorContext(m, t0), e)

	require.Len(t, got, 1)
	assert.Contains(t, got[0].Summary, "references something missing")
}

// When nothing is missing the kubelet rejected the configuration for
// another reason (runAsNonRoot, a bad subPath); the summary says only
// that, and the kubelet's text is quoted as evidence.
func TestConfigErrorWithoutMissingReferenceIsGeneric(t *testing.T) {
	m, e := configErrorFixture(t, "")

	got := Container{}.Detect(testDetectorContext(m, t0), e)

	require.Len(t, got, 1)
	assert.NotContains(t, got[0].Summary, "missing")
	assert.Contains(t, got[0].Summary,
		"the kubelet rejected its configuration")
	var quoted bool
	for _, ev := range got[0].Evidence {
		quoted = quoted || ev.Value == "kubelet said no"
	}
	assert.True(t, quoted, "kubelet message is quoted")
}

// A Secret that exists but lacks a key, or one the inventory has not
// synced yet, still gets the "references something missing" wording:
// the kubelet's own message says so, and it is quoted as evidence.
func TestConfigErrorNamesMissingKeyOrUnsyncedObject(t *testing.T) {
	for _, msg := range []string{
		"couldn't find key token in Secret default/creds",
		`secret "creds" not found`,
		`configmap "settings" not found`,
	} {
		m, e := configErrorFixture(t, "")
		put(m, e.ID, t0, map[string]inventory.Value{
			kube.AttrState:       inventory.Text("waiting"),
			kube.AttrStateReason: inventory.Text(reasons.CreateConfigError),
			kube.AttrMessage:     inventory.Text(msg),
		})
		e, _ = m.Entity(e.ID)

		got := Container{}.Detect(testDetectorContext(m, t0), e)

		require.Len(t, got, 1, msg)
		assert.Contains(t, got[0].Summary,
			"references something missing", msg)
	}
}
