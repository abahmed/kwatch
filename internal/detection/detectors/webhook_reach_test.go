package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// webhookWithRules is webhook(policy, false, nil), a backend Service
// that does not exist, whose fail-closed rules are lines, beside a pod
// in namespace web.
func webhookWithRules(
	policy, lines string,
) (*inventory.Model, inventory.EntityID) {
	m, hook := webhook(policy, false, nil)
	e, _ := m.Entity(hook)
	attrs := map[string]inventory.Value{
		kube.AttrWebhookClosedRules: inventory.Text(lines)}
	for name, a := range e.Attributes {
		attrs[name] = a.Value
	}
	put(m, hook, t0, attrs)
	put(m, newID(kube.KindPod, "web", "api-1"), t0, nil)
	return m, hook
}

func TestWebhookFailingClosedOnExistingObjectsSaysTheyAreRejected(
	t *testing.T,
) {
	m, id := webhookWithRules("Fail",
		"default/hook-svc||pods|*|1||\ndefault/hook-svc||services|*|1||")

	got := evaluate(Webhook{}, m, t0, id, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, reasons.WebhookBackendNotFound, got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity,
		"no request was refused yet")
	assert.Equal(t, "Admission webhook rejects creates and updates of "+
		"pods right now: it fails closed and its backend Service "+
		"hook-svc is gone", got[0].Summary)
	assert.Equal(t, "pods",
		evidenceOf(got[0], detection.EvidenceWebhookIntercepts))
}

func TestWebhookFailingClosedOnGoneResourcesIsOnlyDigest(t *testing.T) {
	m, id := webhookWithRules("Fail",
		"default/hook-svc|widgets.example.com|widgets|*|1||")

	got := evaluate(Webhook{}, m, t0, id, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, detection.Info, got[0].Severity)
	assert.Contains(t, got[0].Summary,
		"its rules match no object that exists now")
	assert.Empty(t, evidenceOf(got[0],
		detection.EvidenceWebhookIntercepts))
}

func TestWebhookWhoseReachIsUnknownSaysSo(t *testing.T) {
	m, id := webhookWithRules("Fail",
		"default/hook-svc|coordination.k8s.io|leases|*|1||")

	got := evaluate(Webhook{}, m, t0, id, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Contains(t, got[0].Summary, "cannot tell whether its rules "+
		"match objects that exist now")
}

// An Ignore webhook is skipped by requests whatever it matches: its
// rules are not judged.
func TestWebhookIgnoringFailureIsNotJudgedByItsRules(t *testing.T) {
	m, id := webhookWithRules("Ignore", "default/hook-svc||pods|*|1||")

	got := evaluate(Webhook{}, m, t0, id, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, detection.Info, got[0].Severity)
	assert.Contains(t, got[0].Summary, "failurePolicy is Ignore")
}

func TestWebhookReachNamesAtMostThreeResources(t *testing.T) {
	assert.Equal(t, "pods", reachNames([]string{"pods"}))
	assert.Equal(t, "pods and services",
		reachNames([]string{"pods", "services"}))
	assert.Equal(t, "a, b, c and 2 more",
		reachNames([]string{"a", "b", "c", "d", "e"}))
}
