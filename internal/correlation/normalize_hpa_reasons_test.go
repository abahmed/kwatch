package correlation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/constant"
)

// One missing-metrics condition arrives under three event reasons; kept apart
// they were up to three alerts per HPA for a single metrics-server outage.
func TestNormalizeReasonFoldsHPAMetricReasons(t *testing.T) {
	assert.Equal(t, constant.ReasonFailedGetResourceMetric,
		normalizeReason(constant.ReasonFailedComputeMetricsReplicas))
	assert.Equal(t, constant.ReasonFailedGetResourceMetric,
		normalizeReason(constant.ReasonFailedGetMetrics))
	assert.Equal(t, constant.ReasonFailedGetResourceMetric,
		normalizeReason(constant.ReasonFailedGetResourceMetric))
	assert.Equal(t, "FailedScheduling", normalizeReason("FailedScheduling"))
}
