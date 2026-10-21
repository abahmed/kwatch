package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A configuration risk is evidence only: next to a real failure it never
// changes the tier, whatever severity the risk carries, and alone it is
// a digest incident at most.
func TestTierIgnoresRiskFindings(t *testing.T) {
	workload := entity(kube.KindDeployment, "api")
	failure := sig(workload, reasons.DeploymentUnavailable,
		detection.Warning)
	for _, risk := range []string{reasons.RiskNoReadinessProbe,
		reasons.RiskNoMemoryLimit, reasons.RiskMutableImageTag,
		reasons.RiskSingleReplica} {
		for _, severity := range []detection.Severity{
			detection.Info, detection.Critical} {
			advisory := sig(workload, risk, severity)
			advisory.Advisory = true

			plain := incidentOf(workload, nil, failure)
			ridden := incidentOf(workload, nil, failure, advisory)
			alone := incidentOf(workload, nil, advisory)

			assert.Equal(t, tier(plain), tier(ridden), risk)
			assert.Equal(t, Digest, tier(alone), risk)
		}
	}
}
