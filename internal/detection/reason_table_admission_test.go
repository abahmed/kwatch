package detection

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The registry's admission reasons and AdmissionReason are one list: a
// reason that is known as an admission state reason is either a
// rejection or the node shutdown.
func TestAdmissionReasonsAreOneList(t *testing.T) {
	for _, info := range admissionReasons {
		if info.Reason == admissionLifecycleReason {
			assert.False(t, AdmissionReason(info.Reason), info.Reason)
			continue
		}
		assert.True(t, AdmissionReason(info.Reason), info.Reason)
	}
}

func TestAdmissionReasonKnowsExtendedResourceShortages(t *testing.T) {
	assert.True(t, AdmissionReason("OutOfnvidia.com/gpu"))
	assert.False(t, AdmissionReason("OutOf"))
	assert.False(t, AdmissionReason("Evicted"))
	assert.False(t, AdmissionReason(""))
}
