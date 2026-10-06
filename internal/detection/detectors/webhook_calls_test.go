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

func hookModel(attrs map[string]inventory.Value) (
	*inventory.Model, inventory.EntityID,
) {
	m := newTestModel()
	id := inventory.CoreID(kube.KindValidatingHook, "", "policy")
	attrs[kube.AttrFailurePolicy] = inventory.Text("Fail")
	put(m, id, t0, attrs)
	return m, id
}

func TestWebhookCallsReportsASlowWebhook(t *testing.T) {
	m, id := hookModel(numbers(kube.AttrWebhookP99, 3200.0,
		kube.AttrWebhookCalls, 90.0,
		kube.AttrWebhookSlowest, "opa.example.com"))

	early := loadFindings(WebhookCalls{}, m, id, 4*time.Minute)
	got := loadFindings(WebhookCalls{}, m, id, hookSlowSustain)

	assert.Empty(t, early)
	require.Len(t, got, 1)
	assert.Equal(t, "WebhookSlow", got[0].Reason)
	assert.Equal(t, detection.Info, got[0].Severity)
	assert.Equal(t, "Admission webhook opa.example.com takes 3.2 seconds per "+
		"call (p99); every request it intercepts waits for it",
		got[0].Summary)
	assert.Contains(t, got[0].Evidence, detection.Evidence{
		Label: "failure policy", Value: "Fail"})
}

func TestWebhookCallsNearTheTimeoutIsAWarning(t *testing.T) {
	m, id := hookModel(numbers(kube.AttrWebhookP99, 8000.0,
		kube.AttrWebhookSlowest, "opa.example.com"))

	got := loadFindings(WebhookCalls{}, m, id, extremeSustain)

	require.Len(t, got, 1)
	assert.Equal(t, detection.Warning, got[0].Severity)
}

func TestWebhookCallsReportsFailingClosedAsAWarning(t *testing.T) {
	m, id := hookModel(numbers(kube.AttrWebhookClosedShare, 40.0,
		kube.AttrWebhookOpenShare, 0.0,
		kube.AttrWebhookSlowest, "opa.example.com"))

	got := loadFindings(WebhookCalls{}, m, id, hookFailSustain)

	require.Len(t, got, 1)
	assert.Equal(t, "WebhookRejecting", got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Equal(t, "Admission webhook opa.example.com fails 40% of its "+
		"calls and fails closed: those requests are refused",
		got[0].Summary)
}

func TestWebhookCallsFailingOpenIsOnlyInformation(t *testing.T) {
	m, id := hookModel(numbers(kube.AttrWebhookClosedShare, 0.0,
		kube.AttrWebhookOpenShare, 100.0,
		kube.AttrWebhookSlowest, "mpod.kb.io"))

	assert.Empty(t, loadFindings(WebhookCalls{}, m, id, hookFailSustain),
		"short of the longer wait")
	got := loadFindings(WebhookCalls{}, m, id, hookSlowSustain)

	require.Len(t, got, 1)
	assert.Equal(t, "WebhookFailingOpen", got[0].Reason)
	assert.Equal(t, detection.Info, got[0].Severity)
	assert.Contains(t, got[0].Summary, "its checks are skipped")
}

func TestWebhookCallsIgnoresAHealthyWebhook(t *testing.T) {
	m, id := hookModel(numbers(kube.AttrWebhookP99, 40.0,
		kube.AttrWebhookClosedShare, 0.0, kube.AttrWebhookOpenShare, 0.0,
		kube.AttrWebhookSlowest, "opa.example.com"))

	assert.Empty(t, loadFindings(WebhookCalls{}, m, id, hookSlowSustain))
}

func TestWebhookCallsIgnoresAConfigurationWithoutMetrics(t *testing.T) {
	m, id := hookModel(map[string]inventory.Value{})

	assert.Empty(t, loadFindings(WebhookCalls{}, m, id, hookSlowSustain))
}
