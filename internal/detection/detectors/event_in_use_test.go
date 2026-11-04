package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const inUseDelete = "Failed deploy model due to operation error: " +
	"DeleteLoadBalancer, api error ResourceInUse: load balancer lb-1 " +
	"cannot be deleted because it is currently associated with another " +
	"service"

// A controller that fails to delete a resource still in use retries it
// forever: the first event is reported, as a warning, with the count the
// Event object kept.
func TestEventFailedDeleteOfAResourceInUseIsReportedAtOnce(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindService, "shop", "web")
	put(m, id, t0, nil)
	noteEntity(m, id, "FailedDeployModel", inUseDelete, 21,
		t0.Add(time.Minute))

	got := Event{}.Detect(
		testDetectorContext(m, t0.Add(2*time.Minute)), entityOf(m, id))

	require.Len(t, got, 1)
	assert.Equal(t, "UnusualEvent.FailedDeployModel", got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Contains(t, got[0].Summary, "still in use")
	assert.Contains(t, got[0].Evidence, detection.Evidence{
		Label: "occurrences", Value: "21"})
}

func TestEventInUseDeleteFailureNeedsBothTheDeleteAndTheRefusal(
	t *testing.T,
) {
	cases := map[string]bool{
		inUseDelete: true,
		"error deleting target group tg-1: ResourceInUse":      true,
		"failed to destroy volume: volume is still attached":   true,
		"deprovisioning failed: DependencyViolation on subnet": true,
		"failed to delete load balancer: throttled, retrying":  false,
		"port 80 is in use": false,
		"created load balancer, associated with another service": false,
	}
	for message, want := range cases {
		assert.Equal(t, want, inUseDeleteFailure(message), message)
	}
}
