package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// riderFacts is a workload whose failing member has the given mode and
// that carries the given risk.
func riderFacts(mode detection.Mode, risk string) caseFacts {
	root := inventory.CoreID(kube.KindDeployment, "shop", "api")
	crash := detection.Finding{
		Entity: inventory.CoreID(kube.KindContainer, "shop", "api-1/app"),
		Reason: reasons.CrashLoopBackOff, Mode: mode,
		Severity: detection.Critical, Since: at(0, 0)}
	advice := detection.Finding{Entity: root, Reason: risk,
		Severity: detection.Info, Advisory: true, Since: at(0, 0)}
	p := incident.Incident{ID: "inc-1", Root: root, Tier: incident.Notify,
		State: incident.Open, Opened: at(0, 0), Revision: 1,
		Members: members(crash, advice)}
	return caseFacts{p: p, now: at(6, 0), members: sortedMembers(p)}
}

// A rider states what is so now: it never says what a risk will do.
func TestRiskRidersStateFactsNotPredictions(t *testing.T) {
	tests := []struct {
		risk string
		mode detection.Mode
		want string
	}{
		{reasons.RiskNoReadinessProbe, detection.ModeProbe,
			"It has no readiness probe, so Kubernetes sends it traffic " +
				"while the container runs."},
		{reasons.RiskNoMemoryLimit, detection.ModeOOMKilled,
			"Its containers have no memory limit, so nothing caps their " +
				"memory use."},
		{reasons.RiskSingleReplica, detection.ModeCrashLoop,
			"It runs a single replica."},
		{reasons.RiskMutableImageTag, detection.ModeCrashLoop,
			"Its image has no fixed tag."},
	}
	for _, tt := range tests {
		t.Run(tt.risk, func(t *testing.T) {
			got := riskSentences(riderFacts(tt.mode, tt.risk))

			if assert.Len(t, got, 1) {
				assert.Equal(t, tt.want, got[0].text)
				assert.False(t, composeForecasts(got[0].text), got[0].text)
				assert.NotContains(t, got[0].text, "downtime")
			}
		})
	}
}
