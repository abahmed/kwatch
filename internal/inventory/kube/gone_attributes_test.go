package kube

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

func goneObservations(
	t *testing.T, obs []inventory.Observation,
) []inventory.Observation {
	t.Helper()
	var out []inventory.Observation
	for _, o := range obs {
		if o.Kind == inventory.Gone {
			out = append(out, o)
		}
	}
	return out
}

// A deleted binding hands over who it bound: the object is gone, and the
// root-cause engine must still tell it from a binding of another account.
func TestTranslatorDeletedBindingKeepsItsAccounts(t *testing.T) {
	translator := NewTranslator(
		NewUnstructuredSchema("rbac.authorization.k8s.io", "RoleBinding"))

	gone := goneObservations(t, translator.Deleted(roleBinding(),
		time.Unix(0, 0)))

	require.Len(t, gone, 1)
	assert.Equal(t, "ops/local widgets/ctl",
		gone[0].Attributes[AttrRBACAccounts].AsText())
	assert.Equal(t, "ClusterRole/widget-ctl",
		gone[0].Attributes[AttrRBACRole].AsText())
}

// Nothing else hands attributes over with its deletion.
func TestTranslatorDeletedPodKeepsNoAttributes(t *testing.T) {
	translator := NewTranslator(PodSchema{})

	for _, o := range goneObservations(t, translator.Deleted(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name: "p", Namespace: "ns"}}, time.Unix(0, 0))) {
		assert.Empty(t, o.Attributes)
	}
}
