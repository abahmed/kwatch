package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestClusterServiceFailingSustained(t *testing.T) {
	tests := []struct {
		id     inventory.EntityID
		reason string
	}{
		{kube.APIServer, reasons.APIServerUnavailable},
		{kube.ClusterDNS, reasons.CoreDNSUnavailable},
		{kube.Etcd, reasons.EtcdUnavailable},
		{kube.Scheduler, reasons.SchedulerUnavailable},
		{kube.ControllerManager,
			reasons.ControllerManagerUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.id.Name, func(t *testing.T) {
			m := newTestModel()
			put(m, tt.id, t0, map[string]inventory.Value{
				kube.AttrHealthy:    inventory.Bool(false),
				kube.AttrProbeError: inventory.Text("refused"),
			})
			early := evaluate(ClusterService{}, m,
				t0.Add(89*time.Second), tt.id, nil)
			assert.Empty(t, early.Findings)
			assert.Equal(t, time.Second, early.RecheckAfter)

			got := evaluate(ClusterService{}, m, t0.Add(90*time.Second),
				tt.id, nil).Findings
			require.Len(t, got, 1)
			assert.Equal(t, tt.reason, got[0].Reason)
			assert.Equal(t, detection.Critical, got[0].Severity)
			assert.NotEmpty(t, got[0].Summary)
			assert.Equal(t, "refused", got[0].Evidence[0].Value)
		})
	}
}

func TestClusterServiceUnknownHealthQuiet(t *testing.T) {
	m := newTestModel()
	put(m, kube.APIServer, t0, nil)
	got := evaluate(ClusterService{}, m, t0.Add(time.Hour),
		kube.APIServer, nil)
	assert.Empty(t, got.Findings)
	assert.Equal(t, "cluster-service", ClusterService{}.Name())
	assert.Len(t, ClusterService{}.Kinds(), 5)
}

func TestClusterServiceAPILatency(t *testing.T) {
	m := newTestModel()
	put(m, kube.APIServer, t0, map[string]inventory.Value{
		kube.AttrHealthy:   inventory.Bool(true),
		kube.AttrLatencyMS: inventory.Number(2000),
	})
	early := evaluate(ClusterService{}, m, t0.Add(time.Minute),
		kube.APIServer, nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, 30*time.Second, early.RecheckAfter)

	got := evaluate(ClusterService{}, m, t0.Add(90*time.Second),
		kube.APIServer, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.APIServerLatency, got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
}

func TestClusterServiceLatencyQuiet(t *testing.T) {
	now := t0.Add(time.Hour)
	m := newTestModel()
	put(m, kube.APIServer, t0, map[string]inventory.Value{
		kube.AttrHealthy:   inventory.Bool(true),
		kube.AttrLatencyMS: inventory.Number(1999),
	})
	assert.Empty(t, evaluate(ClusterService{}, m, now, kube.APIServer,
		nil).Findings)

	put(m, kube.ClusterDNS, t0, map[string]inventory.Value{
		kube.AttrHealthy:   inventory.Bool(true),
		kube.AttrLatencyMS: inventory.Number(9000),
	})
	assert.Empty(t, evaluate(ClusterService{}, m, now, kube.ClusterDNS,
		nil).Findings, "only the API server is judged on latency")
}

func probeEntity(kind inventory.Kind, attrs map[string]inventory.Value,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	id := newID(kind, "default", "checkout")
	put(m, id, t0, attrs)
	return m, id
}

func TestActiveProbeFailureSustained(t *testing.T) {
	tests := []struct {
		kind inventory.Kind
		word string
	}{
		{kube.KindEndpoint, "Probe checkout"},
		{kube.KindService, "Service port"},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			m, id := probeEntity(tt.kind, map[string]inventory.Value{
				kube.AttrHealthy:         inventory.Bool(false),
				kube.AttrFailureDuration: inventory.Number(60),
				kube.AttrProbeError:      inventory.Text("timeout"),
			})
			early := evaluate(ActiveProbe{}, m, t0.Add(59*time.Second),
				id, nil)
			assert.Empty(t, early.Findings)
			assert.Equal(t, time.Second, early.RecheckAfter)

			got := evaluate(ActiveProbe{}, m, t0.Add(time.Minute), id,
				nil).Findings
			require.Len(t, got, 1)
			assert.Equal(t, reasons.ActiveProbeFailure,
				got[0].Reason)
			assert.Equal(t, detection.Critical, got[0].Severity)
			assert.Contains(t, got[0].Summary, tt.word)
			assert.Equal(t, "timeout", got[0].Evidence[0].Value)
		})
	}
}

func TestActiveProbeLatency(t *testing.T) {
	tests := []struct {
		name     string
		latency  float64
		fires    bool
		severity detection.Severity
	}{
		{"quiet under warning", 99, false, 0},
		{"warning", 100, true, detection.Warning},
		{"critical", 500, true, detection.Critical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, id := probeEntity(kube.KindEndpoint,
				map[string]inventory.Value{
					kube.AttrHealthy:       inventory.Bool(true),
					kube.AttrLatencyMS:     inventory.Number(tt.latency),
					kube.AttrLatencyWarnMS: inventory.Number(100),
					kube.AttrLatencyCritMS: inventory.Number(500),
				})
			got := evaluate(ActiveProbe{}, m, t0, id, nil).Findings
			if !tt.fires {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, reasons.ActiveProbeLatency,
				got[0].Reason)
			assert.Equal(t, tt.severity, got[0].Severity)
			assert.NotEmpty(t, got[0].Summary)
		})
	}
}

func TestActiveProbeQuiet(t *testing.T) {
	m, id := probeEntity(kube.KindEndpoint, nil)
	assert.Empty(t, evaluate(ActiveProbe{}, m, t0, id, nil).Findings,
		"unknown health")

	m, id = probeEntity(kube.KindEndpoint, map[string]inventory.Value{
		kube.AttrHealthy: inventory.Bool(true),
	})
	assert.Empty(t, evaluate(ActiveProbe{}, m, t0, id, nil).Findings,
		"no latency")

	m, id = probeEntity(kube.KindEndpoint, map[string]inventory.Value{
		kube.AttrHealthy:   inventory.Bool(true),
		kube.AttrLatencyMS: inventory.Number(9999),
	})
	assert.Empty(t, evaluate(ActiveProbe{}, m, t0, id, nil).Findings,
		"no thresholds configured")
	assert.Equal(t, "active-probe", ActiveProbe{}.Name())
	assert.Len(t, ActiveProbe{}.Kinds(), 3)
}
