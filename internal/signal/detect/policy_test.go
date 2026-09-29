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

func pdb(allowed, healthy, desired, expected float64,
) (*knowledge.Model, knowledge.EntityID) {
	m := newTestModel()
	id := newID(kube.KindPDB, "default", "web")
	put(m, id, t0, map[string]knowledge.Value{
		kube.AttrDisruptionsAllowed: knowledge.Number(allowed),
		kube.AttrCurrentHealthy:     knowledge.Number(healthy),
		kube.AttrDesiredHealthy:     knowledge.Number(desired),
		kube.AttrExpectedPods:       knowledge.Number(expected),
	})
	return m, id
}

func TestBudgetBlockedSustained(t *testing.T) {
	m, id := pdb(0, 1, 2, 3)
	early := evaluate(Budget{}, m, t0.Add(9*time.Minute), id, nil)
	assert.Empty(t, early.Signals)
	assert.Equal(t, time.Minute, early.RecheckAfter)

	late := evaluate(Budget{}, m, t0.Add(10*time.Minute), id, nil)
	require.Len(t, late.Signals, 1)
	assert.Equal(t, constant.ReasonPdbViolation, late.Signals[0].Reason)
	assert.Equal(t, signal.Warning, late.Signals[0].Severity)
	assert.NotEmpty(t, late.Signals[0].Summary)
}

func TestBudgetQuiet(t *testing.T) {
	now := t0.Add(time.Hour)
	tests := []struct {
		name                         string
		allowed, healthy, desired, n float64
	}{
		{"disruptions allowed", 1, 1, 2, 3},
		{"no pods expected", 0, 0, 2, 0},
		{"enough healthy", 0, 2, 2, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, id := pdb(tt.allowed, tt.healthy, tt.desired, tt.n)
			assert.Empty(t, evaluate(Budget{}, m, now, id, nil).Signals)
		})
	}
	assert.Equal(t, "disruption-budget", Budget{}.Name())
	assert.Equal(t, []knowledge.Kind{kube.KindPDB}, Budget{}.Kinds())
}

func TestQuotaExhausted(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindQuota, "default", "q")
	put(m, id, t0, map[string]knowledge.Value{
		kube.AttrExhausted: knowledge.Text("pods,cpu"),
	})
	got := evaluate(Quota{}, m, t0, id, nil).Signals
	require.Len(t, got, 1)
	assert.Equal(t, constant.ReasonResourceQuotaExhausted, got[0].Reason)
	assert.Equal(t, signal.Warning, got[0].Severity)
	assert.Contains(t, got[0].Summary, "pods, cpu")
}

func TestQuotaQuiet(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindQuota, "default", "q")
	put(m, id, t0, nil)
	assert.Empty(t, evaluate(Quota{}, m, t0, id, nil).Signals)
	assert.Equal(t, "quota", Quota{}.Name())
	assert.Equal(t, []knowledge.Kind{kube.KindQuota}, Quota{}.Kinds())
}

func TestAttachmentFailed(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindVolumeAttachment, "", "va")
	put(m, id, t0, map[string]knowledge.Value{
		kube.AttrAttachError: knowledge.Text("timeout"),
	})
	got := evaluate(Attachment{}, m, t0, id, nil).Signals
	require.Len(t, got, 1)
	assert.Equal(t, constant.ReasonVolumeAttachmentFailure, got[0].Reason)
	assert.Equal(t, signal.Critical, got[0].Severity)
	assert.Equal(t, "timeout", got[0].Evidence[0].Value)
	assert.NotEmpty(t, got[0].Summary)
}

func TestAttachmentQuiet(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindVolumeAttachment, "", "va")
	put(m, id, t0, nil)
	assert.Empty(t, evaluate(Attachment{}, m, t0, id, nil).Signals)
	assert.Equal(t, "volume-attachment", Attachment{}.Name())
	assert.Equal(t, []knowledge.Kind{kube.KindVolumeAttachment},
		Attachment{}.Kinds())
}

func webhook(policy string, service bool, ready []float64,
) (*knowledge.Model, knowledge.EntityID) {
	m := newTestModel()
	hook := newID(kube.KindValidatingHook, "", "hook")
	svc := newID(kube.KindService, "default", "hook-svc")
	put(m, hook, t0, map[string]knowledge.Value{
		kube.AttrFailurePolicy: knowledge.Text(policy),
	})
	link(m, hook, knowledge.Serves, svc)
	if service {
		put(m, svc, t0, nil)
	}
	for i, up := range ready {
		slice := newID(kube.KindEndpointSlice, "default",
			"s"+string(rune('a'+i)))
		put(m, slice, t0, map[string]knowledge.Value{
			kube.AttrEndpointsReady: knowledge.Number(up),
		})
		link(m, slice, knowledge.Backs, svc)
	}
	return m, hook
}

func TestWebhookBackendMissing(t *testing.T) {
	tests := []struct {
		policy   string
		severity signal.Severity
	}{
		{"Fail", signal.Critical}, {"Ignore", signal.Warning},
	}
	for _, tt := range tests {
		t.Run(tt.policy, func(t *testing.T) {
			m, id := webhook(tt.policy, false, nil)
			got := evaluate(Webhook{}, m, t0, id, nil).Signals
			require.Len(t, got, 1)
			assert.Equal(t, constant.ReasonWebhookBackendNotFound,
				got[0].Reason)
			assert.Equal(t, tt.severity, got[0].Severity)
			assert.NotEmpty(t, got[0].Summary)
		})
	}
}

func TestWebhookNoReadyEndpoints(t *testing.T) {
	m, id := webhook("Fail", true, []float64{0, 0})
	got := evaluate(Webhook{}, m, t0, id, nil).Signals
	require.Len(t, got, 1)
	assert.Equal(t, constant.ReasonWebhookNoEndpoints, got[0].Reason)
	assert.Equal(t, signal.Critical, got[0].Severity)
}

func TestWebhookQuiet(t *testing.T) {
	m, id := webhook("Fail", true, []float64{0, 1})
	assert.Empty(t, evaluate(Webhook{}, m, t0, id, nil).Signals)

	m, id = webhook("Fail", true, nil)
	assert.Empty(t, evaluate(Webhook{}, m, t0, id, nil).Signals,
		"no slices means unknown, not zero")
	assert.Equal(t, "webhook", Webhook{}.Name())
	assert.Len(t, Webhook{}.Kinds(), 2)
}

func TestWebhookUnsyncedServicesSkipped(t *testing.T) {
	m, id := webhook("Fail", false, nil)
	unsynced := func(k knowledge.Kind) bool { return k != kube.KindService }
	assert.Empty(t, evaluate(Webhook{}, m, t0, id, unsynced).Signals)
}
