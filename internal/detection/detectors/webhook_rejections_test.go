package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// deadWebhook is a webhook configuration of two webhooks whose backend
// Service does not exist, with the policies given.
func deadWebhook(policies string) (*inventory.Model, inventory.EntityID) {
	m, id := webhook("Fail", false, nil)
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrFailurePolicy: inventory.Text(policies),
		kube.AttrWebhookNames:  inventory.Text("a.example.com,b.example.com"),
	})
	return m, id
}

// refuse records the FailedCreate a ReplicaSet gets when the API server
// refuses its pods.
func refuse(m *inventory.Model, at time.Time, message string) {
	rs := newID(kube.KindReplicaSet, "shop", "orders-1")
	put(m, rs, t0, nil)
	warn(m, rs, at, "FailedCreate", "Error creating: "+message)
}

const failedA = `Internal error occurred: failed calling webhook ` +
	`"a.example.com": failed to call webhook: no endpoints available`

func severityOf(t *testing.T, m *inventory.Model, id inventory.EntityID,
	now time.Time,
) detection.Severity {
	t.Helper()
	got := evaluate(Webhook{}, m, now, id, nil).Findings
	require.Len(t, got, 1)
	return got[0].Severity
}

// A dead backend with no refused request is a warning: it pages only
// once a request is refused because of it.
func TestWebhookDeadBackendPagesOnlyWhenARequestIsRefused(t *testing.T) {
	m, id := deadWebhook("Fail,Fail")
	now := t0.Add(time.Minute)
	assert.Equal(t, detection.Warning, severityOf(t, m, id, now))

	refuse(m, now, failedA)
	assert.Equal(t, detection.Critical, severityOf(t, m, id, now))
}

// The refusal counts for a quarter of an hour, and the detector asks to
// be run again when it stops counting.
func TestWebhookRefusalExpires(t *testing.T) {
	m, id := deadWebhook("Fail,Fail")
	refuse(m, t0, failedA)
	ev := evaluate(Webhook{}, m, t0.Add(time.Minute), id, nil)
	require.Len(t, ev.Findings, 1)
	assert.Equal(t, detection.Critical, ev.Findings[0].Severity)
	assert.Positive(t, ev.RecheckAfter)

	later := t0.Add(EventWindow + time.Minute)
	assert.Equal(t, detection.Warning, severityOf(t, m, id, later))
}

// Only a failed call to a webhook of this configuration is a refusal
// caused by it: another webhook, a denial and a webhook that fails open
// are not.
func TestWebhookRefusalMustNameAFailClosedWebhookOfIt(t *testing.T) {
	tests := map[string]struct {
		policies, message string
	}{
		"other webhook": {"Fail,Fail", `failed calling webhook ` +
			`"other.example.com": no endpoints available`},
		"denied": {"Fail,Fail", `admission webhook "a.example.com" ` +
			`denied the request: no latest tag`},
		"fails open": {"Ignore,Fail", failedA},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m, id := deadWebhook(tt.policies)
			refuse(m, t0, tt.message)
			assert.Equal(t, detection.Warning,
				severityOf(t, m, id, t0.Add(time.Minute)))
		})
	}
}

// The API server's own count of calls that failed closed is a refusal
// too, whatever the controllers were told.
func TestWebhookClosedCallsAreRefusals(t *testing.T) {
	m, id := deadWebhook("Fail,Fail")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrFailurePolicy:      inventory.Text("Fail,Fail"),
		kube.AttrWebhookNames:       inventory.Text("a.example.com"),
		kube.AttrWebhookClosedShare: inventory.Number(0),
	})
	assert.Equal(t, detection.Warning,
		severityOf(t, m, id, t0.Add(time.Minute)))

	put(m, id, t0, map[string]inventory.Value{
		kube.AttrFailurePolicy:      inventory.Text("Fail,Fail"),
		kube.AttrWebhookClosedShare: inventory.Number(40),
	})
	assert.Equal(t, detection.Critical,
		severityOf(t, m, id, t0.Add(time.Minute)))
}

func TestIsWebhookRejection(t *testing.T) {
	note := inventory.Note{Warning: true, Message: failedA}
	assert.True(t, IsWebhookRejection(note))
	note.Warning = false
	assert.False(t, IsWebhookRejection(note))
	assert.False(t, IsWebhookRejection(inventory.Note{
		Warning: true, Message: "exceeded quota: compute"}))
}
