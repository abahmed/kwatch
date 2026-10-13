package explain

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Admission goes only through admissionChecks: the dispatch sites call
// the table, and never a kind's own function.
func TestAdmissionIsDispatchedOnlyByTheTable(t *testing.T) {
	want := map[string][]string{
		"virtual.go":    {"allAdmitHops"},
		"view_modes.go": {"refusalModes"},
		"view.go":       {"admitsElsewhere"},
	}
	banned := []string{"otherWebhook", "otherQuota", "admissionModes(",
		"podSecurityModes", "limitRangeModes", "policyModes"}
	for file, calls := range want {
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		src := string(data)
		for _, call := range calls {
			assert.Contains(t, src, call, file)
		}
		for _, name := range banned {
			assert.NotContains(t, src, name, file)
		}
	}
}

func TestAdmissionChecksCoverEveryRefuser(t *testing.T) {
	for _, kind := range append([]inventory.Kind{kube.KindQuota,
		kube.KindNamespace, kube.KindLimitRange, kindAdmissionPolicy},
		admitKinds...) {
		_, ok := admissionCheckFor(kind)
		assert.True(t, ok, kind)
	}
	_, ok := admissionCheckFor(kube.KindPod)
	assert.False(t, ok)

}
