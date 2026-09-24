package insight

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/model"
)

func TestAvailabilitySeverityAndImpactDistinguishPartialOutage(t *testing.T) {
	e := newTestEngine(nil, nil)
	partial := &model.Incident{Subject: model.Subject{
		Key: "deployment/apps/api", Resource: "deployment", Namespace: "apps",
		Name: "api",
	}, Status: model.Status{Severity: model.SeverityNormal},
		Evidence: model.Evidence{
			Facts: model.Facts{DesiredReplicas: 3, ReadyReplicas: 2},
		}}
	ins := e.Analyze(partial)
	require.Equal(t, model.SeverityWarning, ins.Severity)
	require.Equal(
		t, "2/3 replicas are ready, reducing workload capacity", ins.Impact,
	)

	partial.Facts.ReadyReplicas = 0
	ins = e.Analyze(partial)
	require.Equal(t, model.SeverityCritical, ins.Severity)
	require.Equal(t, "all 3 desired replicas are unavailable", ins.Impact)
}

func TestAvailabilityImpactIncludesZeroHealthyEndpoints(t *testing.T) {
	e := NewEngineWithClock(nil, nil, clock.RealClock{})
	inc := &model.Incident{Subject: model.Subject{
		Key: "service/apps/api", Resource: "service", Namespace: "apps",
		Name: "api",
	}, Status: model.Status{Severity: model.SeverityWarning},
		Evidence: model.Evidence{
			Facts: model.Facts{EndpointsObserved: true, HealthyEndpoints: 0},
		}}

	ins := e.Analyze(inc)
	require.Equal(t, model.SeverityCritical, ins.Severity)
	require.Equal(t,
		"the service has 0 healthy endpoints and cannot receive traffic",
		ins.Impact,
	)
}
