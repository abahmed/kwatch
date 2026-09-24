package insight

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

func TestMetricsAPIInspectorClassifiesUnavailableStates(t *testing.T) {
	cases := []struct {
		name        string
		evidence    MetricsAPIEvidence
		wantPattern string
		wantCause   string
	}{
		{
			name:        "not registered",
			evidence:    MetricsAPIEvidence{Observed: true},
			wantPattern: "metrics_api_failure",
			wantCause:   "not registered",
		},
		{
			name: "no ready endpoints",
			evidence: MetricsAPIEvidence{
				Observed: true, Registered: true, Available: false,
				EndpointsSeen: true, Service: model.ObjectRef{
					Kind: "Service", Namespace: "monitoring", Name: "metrics",
				},
			},
			wantPattern: "metrics_api_failure",
			wantCause:   "no healthy endpoints",
		},
		{
			name: "service unavailable",
			evidence: MetricsAPIEvidence{
				Observed: true, Registered: true, Available: false,
			},
			wantPattern: "metrics_api_failure",
			wantCause:   "unavailable",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			e := NewEngineWithDependencies(nil, nil, Dependencies{
				Clock: clock.RealClock{},
				MetricsAPIInspector: func() MetricsAPIEvidence {
					return test.evidence
				},
			})
			inc := &model.Incident{Subject: model.Subject{
				Key: "hpa/apps/api", Resource: "horizontalpodautoscaler",
				Namespace: "apps", Name: "api",
				Reason: constant.ReasonFailedGetMetrics,
			}}
			ins := e.Analyze(inc)
			require.Equal(t, test.wantPattern, ins.Pattern)
			require.Contains(t, ins.Cause, test.wantCause)
		})
	}
}

func TestMetricsAPIAvailabilityContradictsOnlyGenericFailure(t *testing.T) {
	e := NewEngineWithDependencies(nil, nil, Dependencies{
		Clock: clock.RealClock{},
		MetricsAPIInspector: func() MetricsAPIEvidence {
			return MetricsAPIEvidence{
				Observed: true, Registered: true, Available: true,
			}
		},
	})
	inc := &model.Incident{Subject: model.Subject{
		Key: "hpa/apps/api", Resource: "horizontalpodautoscaler",
		Namespace: "apps", Name: "api",
		Reason: constant.ReasonFailedGetMetrics,
	}}
	ins := e.Analyze(inc)
	require.Contains(t, ins.Contradictions,
		"the Metrics APIService is currently available")
}
