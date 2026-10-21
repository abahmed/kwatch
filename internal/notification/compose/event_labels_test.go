package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// An event says what happened, not what the object can do now: "failed
// to deploy" reports the event, "can't deploy" claims a state.
func TestLoadBalancerEventLabelsReportWhatHappened(t *testing.T) {
	got := shortWhat("Kubernetes reported FailedDeployModel 3 times in 15 minutes")

	assert.Contains(t, got, "failed to deploy its load balancer")
	assert.NotContains(t, got, "can't")
}
