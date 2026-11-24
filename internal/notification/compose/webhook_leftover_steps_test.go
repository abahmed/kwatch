package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// leftoverHook is validating webhook guard with one finding, explained
// at score.
func leftoverHook(
	reason, summary string, score float64,
) (incident.Incident, []detection.Finding) {
	hook := inventory.CoreID(kube.KindValidatingHook, "", "guard")
	m := detection.Finding{Entity: hook, Reason: reason,
		Severity: detection.Warning, Summary: summary}
	return incident.Incident{Root: hook, Members: members(m),
		Cause: &rootcause.CauseRecord{Rule: "self", Root: hook,
			Score: score}}, []detection.Finding{m}
}

// A webhook configuration nothing serves gets a fix: see the Service it
// calls, then delete it, as a labelled change, when the cause is at
// least likely.
func TestLeftoverWebhookGetsADeleteStep(t *testing.T) {
	for name, tc := range map[string]struct {
		reason, summary string
		score           float64
		want            bool
	}{
		"service gone, rules match nothing": {
			reasons.WebhookBackendNotFound, "Admission webhook calls " +
				"Service guard-svc, which does not exist; its rules " +
				"match no object that exists now (failurePolicy Fail)",
			0.95, true},
		"service gone, nothing says it is unneeded": {
			reasons.WebhookBackendNotFound, "Admission webhook calls " +
				"Service guard-svc, which does not exist; no request " +
				"has been rejected yet (failurePolicy Fail)", 0.95, false},
		"no pods, rules match nothing": {reasons.WebhookNoEndpoints,
			"Admission webhook backend guard-svc has no ready pods; its " +
				"rules match no object that exists now (failurePolicy " +
				"Fail)", 0.95, true},
		"no pods, rules match objects": {reasons.WebhookNoEndpoints,
			"Admission webhook backend guard-svc has no ready pods; no " +
				"request has been rejected yet (failurePolicy Fail)",
			0.95, false},
		"cause only possible": {reasons.WebhookBackendNotFound,
			"Admission webhook calls Service guard-svc, which does not " +
				"exist; its rules match no object that exists now " +
				"(failurePolicy Fail)", 0.3, false},
	} {
		p, found := leftoverHook(tc.reason, tc.summary, tc.score)
		steps := nextSteps(p, found)
		last := steps[len(steps)-1]
		deletes := last.Command ==
			"kubectl delete validatingwebhookconfiguration guard"
		assert.Equal(t, tc.want, deletes, name)
		if deletes {
			assert.True(t, last.Mutating, name)
			assert.Contains(t, last.Text, "its checks or changes stop "+
				"applying; reinstall the controller instead if it is "+
				"still wanted", name)
			assert.Equal(t, "kubectl get validatingwebhookconfiguration "+
				"guard -o jsonpath='{.webhooks[*].clientConfig.service}'",
				steps[len(steps)-2].Command, name)
		}
	}
}

// The webhook detector marks a dead backend's finding a leftover of the
// controller's absent backend when it blocks nothing: that is evidence
// enough to offer the delete without the rules-match-nothing words.
func TestLeftoverMarkedWebhookGetsADeleteStep(t *testing.T) {
	p, found := leftoverHook(reasons.WebhookBackendNotFound,
		"Admission webhook calls Service guard-svc, which does not "+
			"exist; requests skip it because failurePolicy is Ignore",
		0.95)
	steps := nextSteps(p, found)
	assert.NotEqual(t, "kubectl delete validatingwebhookconfiguration "+
		"guard", steps[len(steps)-1].Command, "not marked")

	found[0].Leftover = inventory.CoreID(kube.KindDeployment, "ops",
		"guard-operator")
	p.Members = members(found[0])
	steps = nextSteps(p, found)
	assert.Equal(t, "kubectl delete validatingwebhookconfiguration guard",
		steps[len(steps)-1].Command, "marked a leftover")
}
