package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJobFailedSummaryCountsPodFailures(t *testing.T) {
	assert.Equal(t, "Job failed", jobFailedSummary(0))
	assert.Equal(t, "Job failed after 1 pod failure", jobFailedSummary(1))
	assert.Equal(t, "Job failed after 3 pod failures", jobFailedSummary(3))
}
