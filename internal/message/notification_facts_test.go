package message

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

func TestNotificationStoryUsesMeasuredCaseFacts(t *testing.T) {
	cases := []struct {
		name   string
		reason string
		facts  model.Facts
		want   string
	}{
		{
			name:   "service degraded but serving",
			reason: constant.ReasonServiceBackendsDegraded,
			facts: model.Facts{
				BackendPods: 3, UnreadyBackendPods: 1,
				HealthyEndpoints: 2, EndpointsObserved: true,
			},
			want: "⚠️ 1/3 selected backend pods are unready; " +
				"the Service still has 2 ready endpoints.",
		},
		{
			name:   "service selector has no matching pods",
			reason: constant.ReasonServiceNoEndpoints,
			facts: model.Facts{
				EndpointsObserved: true, BackendsObserved: true,
			},
			want: "no pods currently match its selector",
		},
		{
			name:   "service outage with backend evidence",
			reason: constant.ReasonServiceNoEndpoints,
			facts: model.Facts{
				EndpointsObserved: true, BackendPods: 3,
				UnreadyBackendPods: 3,
			},
			want: "🚨 Service has no ready endpoints; 3/3 selected " +
				"backend pods are unready.",
		},
		{
			name:   "deployment availability",
			reason: constant.ReasonDeploymentAvailable,
			facts:  model.Facts{DesiredReplicas: 3, ReadyReplicas: 1},
			want:   "📊 1/3 replicas are ready.",
		},
		{
			name:   "service port mismatch",
			reason: constant.ReasonServicePortMismatch,
			facts: model.Facts{
				MissingServicePortKind:  "target port",
				MissingServicePortValue: "TCP/8080",
			},
			want: "🔌 EndpointSlices do not publish target port " +
				"\"TCP/8080\".",
		},
		{
			name:   "HPA metric",
			reason: constant.ReasonFailedGetResourceMetric,
			facts:  model.Facts{MetricName: "memory"},
			want:   "📊 Affected metric: memory.",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			report := &Report{
				Action: "create", Reason: test.reason, Facts: test.facts,
				Diagnosis: &DiagnosisSection{},
			}
			inc := &model.Incident{}
			n := NotificationFromReport(report, inc, "", 0, 0)
			require.NotNil(t, n)
			assert.Contains(t, n.Summary.Story, test.want)
			assert.Contains(t, n.Summary.Story,
				"🔎 Cause not confirmed yet.")
		})
	}
}

func TestNotificationEvidenceIsBounded(t *testing.T) {
	report := &Report{
		Action: "create",
		Diagnosis: &DiagnosisSection{
			Cause: "the node is not ready", Confidence: 0.99,
			CauseState: "confirmed",
			Evidence: []string{"node condition is false", "pods are unready",
				"third signal"},
		},
	}
	n := NotificationFromReport(report, &model.Incident{}, "", 0, 0)
	require.NotNil(t, n)
	require.Len(t, n.Details, 1)
	assert.Equal(t, "🧾 Evidence", n.Details[0].Title)
	assert.Equal(t, []string{
		"node condition is false", "pods are unready",
	}, n.Details[0].Lines)
}

func TestNotificationShowsObservationsWithoutClaimingCause(t *testing.T) {
	report := &Report{
		Action: "create",
		Diagnosis: &DiagnosisSection{
			Evidence: []string{"HPA could not obtain metrics"},
		},
	}
	n := NotificationFromReport(report, &model.Incident{}, "", 0, 0)
	require.NotNil(t, n)
	assert.Contains(t, n.Summary.Story, "Cause not confirmed yet")
	require.Len(t, n.Details, 1)
	assert.Equal(t, "🧾 Observed", n.Details[0].Title)
}
