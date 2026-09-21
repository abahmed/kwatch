package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestNormalizeReasonCoversRetryAndMetricAliases(t *testing.T) {
	for _, test := range []struct {
		raw, want string
	}{
		{constant.ReasonErrImagePull, constant.ReasonImagePullBackOff},
		{"ErrImagePull 5", constant.ReasonImagePullBackOff},
		{"ImagePullBackOff 5", constant.ReasonImagePullBackOff},
		{"BackOff 5", constant.ReasonBackOff},
		{constant.ReasonFailedGetMetrics, constant.ReasonFailedGetResourceMetric},
		{constant.ReasonFailedComputeMetricsReplicas,
			constant.ReasonFailedGetResourceMetric},
	} {
		assert.Equal(t, test.want, normalizeReason(test.raw))
	}
}

func TestProcessStoresCanonicalReasonAndStableID(t *testing.T) {
	e := newTestEngine()
	first, action := e.processEvent(event.Event{
		Resource: "hpa", Namespace: "prod", PodName: "api",
		Reason: constant.ReasonFailedGetMetrics,
	}, "api", nil)
	require.NotNil(t, first)
	assert.Equal(t, model.ActionCreate, action)
	assert.Equal(t, constant.ReasonFailedGetResourceMetric, first.Reason)
	assert.NotEmpty(t, first.ID)

	second, _ := e.processEvent(event.Event{
		Resource: "hpa", Namespace: "prod", PodName: "api",
		Reason: constant.ReasonFailedComputeMetricsReplicas,
	}, "api", nil)
	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, constant.ReasonFailedGetResourceMetric, second.Reason)
}

func TestRestoreIncidentRecordsMergesCanonicalCollision(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	records := []model.PersistedIncident{
		{
			Key:       BuildKey("prod", "api", "FailedGetMetrics", ""),
			Reason:    constant.ReasonFailedGetMetrics,
			Namespace: "prod", Name: "api", Resource: "hpa", Count: 2,
			FirstSeen: now, LastSeen: now,
			Resources: map[string]bool{"api-1": true}, State: model.StateResolved,
		},
		{
			Key:       BuildKey("prod", "api", "FailedGetResourceMetric", ""),
			Reason:    constant.ReasonFailedGetResourceMetric,
			Namespace: "prod", Name: "api", Resource: "hpa", Count: 5,
			FirstSeen: now.Add(time.Minute), LastSeen: now.Add(2 * time.Minute),
			Resources: map[string]bool{"api-2": true}, State: model.StateActive,
		},
	}

	incidents, aliases := RestoreIncidentRecords(records)
	key := BuildKey("prod", "api", constant.ReasonFailedGetResourceMetric, "")
	merged := incidents[key]
	require.NotNil(t, merged)
	assert.Equal(t, model.StateActive, merged.State)
	assert.Equal(t, 5, merged.Count)
	assert.Equal(t, now, merged.FirstSeen)
	assert.Equal(t, now.Add(2*time.Minute), merged.LastSeen)
	assert.True(t, merged.Resources["api-1"])
	assert.True(t, merged.Resources["api-2"])
	assert.Equal(t, key, aliases[records[0].Key])
	assert.Equal(t, merged.ID, incidentID(key))
}

func TestSharedMetricsFailureUsesGlobalGroupOnlyWithEvidence(t *testing.T) {
	metricsFailure := event.Event{
		Reason: constant.ReasonFailedGetResourceMetric,
		Message: "unable to fetch metrics from metrics.k8s.io: server currently " +
			"unable to handle the request",
	}
	key := computeGroupKey(
		constant.ReasonFailedGetResourceMetric,
		metricsFailure, "api", "",
	)
	assert.Equal(
		t,
		constant.ReasonFailedGetResourceMetric+"|global|metrics-api",
		key,
	)
	invalid := metricsFailure
	invalid.Message = "invalid target reference"
	invalid.Namespace = "prod"
	assert.Equal(
		t,
		constant.ReasonFailedGetResourceMetric+"|prod|api",
		computeGroupKey(
			constant.ReasonFailedGetResourceMetric,
			invalid, "api", "",
		),
	)
}
