package detect

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestClusterServiceFailingSustained(t *testing.T) {
	tests := []struct {
		id     knowledge.EntityID
		reason string
	}{
		{kube.APIServer, constant.ReasonAPIServerUnavailable},
		{kube.ClusterDNS, constant.ReasonCoreDNSUnavailable},
		{kube.Etcd, constant.ReasonEtcdUnavailable},
		{kube.Scheduler, constant.ReasonSchedulerUnavailable},
		{kube.ControllerManager,
			constant.ReasonControllerManagerUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.id.Name, func(t *testing.T) {
			m := newTestModel()
			put(m, tt.id, t0, map[string]knowledge.Value{
				kube.AttrHealthy:    knowledge.Bool(false),
				kube.AttrProbeError: knowledge.Text("refused"),
			})
			early := evaluate(ClusterService{}, m,
				t0.Add(89*time.Second), tt.id, nil)
			assert.Empty(t, early.Signals)
			assert.Equal(t, time.Second, early.RecheckAfter)

			got := evaluate(ClusterService{}, m, t0.Add(90*time.Second),
				tt.id, nil).Signals
			require.Len(t, got, 1)
			assert.Equal(t, tt.reason, got[0].Reason)
			assert.Equal(t, signal.Critical, got[0].Severity)
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
	assert.Empty(t, got.Signals)
	assert.Equal(t, "cluster-service", ClusterService{}.Name())
	assert.Len(t, ClusterService{}.Kinds(), 5)
}

func TestClusterServiceAPILatency(t *testing.T) {
	m := newTestModel()
	put(m, kube.APIServer, t0, map[string]knowledge.Value{
		kube.AttrHealthy:   knowledge.Bool(true),
		kube.AttrLatencyMS: knowledge.Number(2000),
	})
	early := evaluate(ClusterService{}, m, t0.Add(time.Minute),
		kube.APIServer, nil)
	assert.Empty(t, early.Signals)
	assert.Equal(t, 30*time.Second, early.RecheckAfter)

	got := evaluate(ClusterService{}, m, t0.Add(90*time.Second),
		kube.APIServer, nil).Signals
	require.Len(t, got, 1)
	assert.Equal(t, constant.ReasonAPIServerLatency, got[0].Reason)
	assert.Equal(t, signal.Warning, got[0].Severity)
}

func TestClusterServiceLatencyQuiet(t *testing.T) {
	now := t0.Add(time.Hour)
	m := newTestModel()
	put(m, kube.APIServer, t0, map[string]knowledge.Value{
		kube.AttrHealthy:   knowledge.Bool(true),
		kube.AttrLatencyMS: knowledge.Number(1999),
	})
	assert.Empty(t, evaluate(ClusterService{}, m, now, kube.APIServer,
		nil).Signals)

	put(m, kube.ClusterDNS, t0, map[string]knowledge.Value{
		kube.AttrHealthy:   knowledge.Bool(true),
		kube.AttrLatencyMS: knowledge.Number(9000),
	})
	assert.Empty(t, evaluate(ClusterService{}, m, now, kube.ClusterDNS,
		nil).Signals, "only the API server is judged on latency")
}

func probeEntity(kind knowledge.Kind, attrs map[string]knowledge.Value,
) (*knowledge.Model, knowledge.EntityID) {
	m := newTestModel()
	id := newID(kind, "default", "checkout")
	put(m, id, t0, attrs)
	return m, id
}

func TestActiveProbeFailureSustained(t *testing.T) {
	tests := []struct {
		kind knowledge.Kind
		word string
	}{
		{kube.KindEndpoint, "Probe checkout"},
		{kube.KindService, "Service port"},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			m, id := probeEntity(tt.kind, map[string]knowledge.Value{
				kube.AttrHealthy:         knowledge.Bool(false),
				kube.AttrFailureDuration: knowledge.Number(60),
				kube.AttrProbeError:      knowledge.Text("timeout"),
			})
			early := evaluate(ActiveProbe{}, m, t0.Add(59*time.Second),
				id, nil)
			assert.Empty(t, early.Signals)
			assert.Equal(t, time.Second, early.RecheckAfter)

			got := evaluate(ActiveProbe{}, m, t0.Add(time.Minute), id,
				nil).Signals
			require.Len(t, got, 1)
			assert.Equal(t, constant.ReasonActiveProbeFailure,
				got[0].Reason)
			assert.Equal(t, signal.Critical, got[0].Severity)
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
		severity signal.Severity
	}{
		{"quiet under warning", 99, false, 0},
		{"warning", 100, true, signal.Warning},
		{"critical", 500, true, signal.Critical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, id := probeEntity(kube.KindEndpoint,
				map[string]knowledge.Value{
					kube.AttrHealthy:       knowledge.Bool(true),
					kube.AttrLatencyMS:     knowledge.Number(tt.latency),
					kube.AttrLatencyWarnMS: knowledge.Number(100),
					kube.AttrLatencyCritMS: knowledge.Number(500),
				})
			got := evaluate(ActiveProbe{}, m, t0, id, nil).Signals
			if !tt.fires {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, constant.ReasonActiveProbeLatency,
				got[0].Reason)
			assert.Equal(t, tt.severity, got[0].Severity)
			assert.NotEmpty(t, got[0].Summary)
		})
	}
}

func TestActiveProbeQuiet(t *testing.T) {
	m, id := probeEntity(kube.KindEndpoint, nil)
	assert.Empty(t, evaluate(ActiveProbe{}, m, t0, id, nil).Signals,
		"unknown health")

	m, id = probeEntity(kube.KindEndpoint, map[string]knowledge.Value{
		kube.AttrHealthy: knowledge.Bool(true),
	})
	assert.Empty(t, evaluate(ActiveProbe{}, m, t0, id, nil).Signals,
		"no latency")

	m, id = probeEntity(kube.KindEndpoint, map[string]knowledge.Value{
		kube.AttrHealthy:   knowledge.Bool(true),
		kube.AttrLatencyMS: knowledge.Number(9999),
	})
	assert.Empty(t, evaluate(ActiveProbe{}, m, t0, id, nil).Signals,
		"no thresholds configured")
	assert.Equal(t, "active-probe", ActiveProbe{}.Name())
	assert.Len(t, ActiveProbe{}.Kinds(), 2)
}
