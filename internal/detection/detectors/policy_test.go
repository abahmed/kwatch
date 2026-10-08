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

func pdb(allowed, healthy, desired, expected float64,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	id := newID(kube.KindPDB, "default", "web")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrDisruptionsAllowed: inventory.Number(allowed),
		kube.AttrCurrentHealthy:     inventory.Number(healthy),
		kube.AttrDesiredHealthy:     inventory.Number(desired),
		kube.AttrExpectedPods:       inventory.Number(expected),
	})
	return m, id
}

func TestBudgetBlockedSustained(t *testing.T) {
	m, id := pdb(0, 1, 2, 3)
	early := evaluate(Budget{}, m, t0.Add(9*time.Minute), id, nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, time.Minute, early.RecheckAfter)

	late := evaluate(Budget{}, m, t0.Add(10*time.Minute), id, nil)
	require.Len(t, late.Findings, 1)
	assert.Equal(t, reasons.PdbViolation, late.Findings[0].Reason)
	assert.Equal(t, detection.Warning, late.Findings[0].Severity)
	assert.NotEmpty(t, late.Findings[0].Summary)
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
			assert.Empty(t, evaluate(Budget{}, m, now, id, nil).Findings)
		})
	}
	assert.Equal(t, "disruption-budget", Budget{}.Name())
	assert.Equal(t, []inventory.Kind{kube.KindPDB}, Budget{}.Kinds())
}

func TestQuotaExhausted(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindQuota, "default", "q")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrExhausted: inventory.Text("pods,cpu"),
	})
	rs := newID(kube.KindReplicaSet, "default", "web-1")
	put(m, rs, t0, nil)
	warn(m, rs, t0, "FailedCreate",
		`Error creating: pods "web-1-x" is forbidden: exceeded quota: q`)
	got := evaluate(Quota{}, m, t0.Add(time.Minute), id, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.ResourceQuotaExhausted, got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Contains(t, got[0].Summary, "pods, cpu")
}

// A quota used up to exactly its limit is normal when nothing more is
// being created; only a refused create makes it a problem.
func TestQuotaAtTheLimitIsInfoUntilACreateIsRefused(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindQuota, "default", "q")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrExhausted: inventory.Text("pods"),
	})

	got := evaluate(Quota{}, m, t0, id, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, detection.Info, got[0].Severity)
}

func TestQuotaQuiet(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindQuota, "default", "q")
	put(m, id, t0, nil)
	assert.Empty(t, evaluate(Quota{}, m, t0, id, nil).Findings)
	assert.Equal(t, "quota", Quota{}.Name())
	assert.Equal(t, []inventory.Kind{kube.KindQuota}, Quota{}.Kinds())
}

func TestAttachmentFailed(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindVolumeAttachment, "", "va")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrAttachError: inventory.Text("timeout"),
	})
	// A throttled or retried attach clears itself within moments.
	early := evaluate(Attachment{}, m, t0.Add(time.Second), id, nil)
	assert.Empty(t, early.Findings)
	assert.Positive(t, early.RecheckAfter)
	got := evaluate(Attachment{}, m, t0.Add(DefaultCustomFailing), id,
		nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.VolumeAttachmentFailure, got[0].Reason)
	assert.Equal(t, detection.Critical, got[0].Severity)
	assert.Equal(t, "timeout", got[0].Evidence[0].Value)
	assert.NotEmpty(t, got[0].Summary)
}

func TestAttachmentQuiet(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindVolumeAttachment, "", "va")
	put(m, id, t0, nil)
	assert.Empty(t, evaluate(Attachment{}, m, t0, id, nil).Findings)
	assert.Equal(t, "volume-attachment", Attachment{}.Name())
	assert.Equal(t, []inventory.Kind{kube.KindVolumeAttachment},
		Attachment{}.Kinds())
}

func webhook(policy string, service bool, ready []float64,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	hook := newID(kube.KindValidatingHook, "", "hook")
	svc := newID(kube.KindService, "default", "hook-svc")
	put(m, hook, t0, map[string]inventory.Value{
		kube.AttrFailurePolicy: inventory.Text(policy),
	})
	link(m, hook, inventory.Serves, svc)
	if service {
		put(m, svc, t0, nil)
	}
	for i, up := range ready {
		slice := newID(kube.KindEndpointSlice, "default",
			"s"+string(rune('a'+i)))
		put(m, slice, t0, map[string]inventory.Value{
			kube.AttrEndpointsReady: inventory.Number(up),
		})
		link(m, slice, inventory.Backs, svc)
	}
	return m, hook
}

func TestWebhookBackendMissing(t *testing.T) {
	tests := []struct {
		policy   string
		severity detection.Severity
	}{
		{"Fail", detection.Warning}, {"Ignore", detection.Info},
		{"Ignore,Fail", detection.Warning},
	}
	for _, tt := range tests {
		t.Run(tt.policy, func(t *testing.T) {
			m, id := webhook(tt.policy, false, nil)
			got := evaluate(Webhook{}, m, t0, id, nil).Findings
			require.Len(t, got, 1)
			assert.Equal(t, reasons.WebhookBackendNotFound,
				got[0].Reason)
			assert.Equal(t, tt.severity, got[0].Severity)
			assert.NotEmpty(t, got[0].Summary)
		})
	}
}

func TestWebhookNoReadyEndpoints(t *testing.T) {
	m, id := webhook("Fail", true, []float64{0, 0})
	got := evaluate(Webhook{}, m, t0, id, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.WebhookNoEndpoints, got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity,
		"nothing was refused yet")
}

func TestWebhookNoEndpointsIgnoreIsInfo(t *testing.T) {
	m, id := webhook("Ignore", true, []float64{0})
	got := evaluate(Webhook{}, m, t0, id, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, detection.Info, got[0].Severity)
	assert.Contains(t, got[0].Summary, "failurePolicy is Ignore")

	m, id = webhook("Fail", true, []float64{0})
	got = evaluate(Webhook{}, m, t0, id, nil).Findings
	assert.Contains(t, got[0].Summary, "no request has been rejected yet")
}

func TestWebhookQuiet(t *testing.T) {
	m, id := webhook("Fail", true, []float64{0, 1})
	assert.Empty(t, evaluate(Webhook{}, m, t0, id, nil).Findings)

	m, id = webhook("Fail", true, nil)
	assert.Empty(t, evaluate(Webhook{}, m, t0, id, nil).Findings,
		"no slices means unknown, not zero")
	assert.Equal(t, "webhook", Webhook{}.Name())
	assert.Len(t, Webhook{}.Kinds(), 2)
}

func TestWebhookUnsyncedServicesSkipped(t *testing.T) {
	m, id := webhook("Fail", false, nil)
	unsynced := func(k inventory.Kind) bool { return k != kube.KindService }
	assert.Empty(t, evaluate(Webhook{}, m, t0, id, unsynced).Findings)
}
