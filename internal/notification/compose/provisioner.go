package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// weightProvisioner ranks what runs a claim's provisioner with the other
// proof of a claim that cannot be provisioned.
const weightProvisioner = 0.65

// provisionerNone is the evidence value of a provisioner no workload
// serves.
const provisionerNone = "none found"

// provisionerRules are the explain rows that blame the workload serving
// a claim's provisioner.
var provisionerRules = []string{"provisioner-down", "provisioner-down-pending"}

// noReadyPods ends the evidence of a serving workload with no ready pod.
const noReadyPods = ", no ready pods"

// provisionerClaim is the Pending claim whose evidence names its
// provisioner and what runs it.
func provisionerClaim(f caseFacts) (detection.Finding, bool) {
	for _, m := range f.members {
		if evidence(m, detection.EvidenceProvisioner) != "" &&
			evidence(m, detection.EvidenceProvisionerWorkload) != "" {
			return m, true
		}
	}
	return detection.Finding{}, false
}

// provisionerLead leads with the Pending claim and the workload that
// should provision its volume: "Volume claim data-0 in shop is Pending:
// its provisioner ebs.csi.example.com runs as deployment kube-system/
// ebs-csi-controller, which has no ready pods".
func provisionerLead(f caseFacts) (string, bool) {
	claim, ok := provisionerClaim(f)
	if !ok || !causeRuleIs(f, provisionerRules...) {
		return "", false
	}
	runs := evidence(claim, detection.EvidenceProvisionerWorkload)
	workload, down := strings.CutSuffix(runs, noReadyPods)
	if !down {
		return "", false
	}
	return f.leadName(claim.Entity) + " is Pending: its " +
		"provisioner " + evidence(claim, detection.EvidenceProvisioner) +
		" runs as " + workload + ", which has no ready pods", true
}

// provisionerSentences say, from a Pending claim's evidence, that kwatch
// sees no workload serving its provisioner, and quote the provisioning
// failure. A serving workload
// with no ready pod is the lead's business (provisionerLead) when it is
// blamed, and a proof sentence otherwise.
func provisionerSentences(f caseFacts) []sentence {
	claim, ok := provisionerClaim(f)
	if !ok || causeRuleIs(f, provisionerRules...) {
		return nil
	}
	runs := evidence(claim, detection.EvidenceProvisionerWorkload)
	subject := "Claim " + claim.Entity.Name + " is Pending: its " +
		"provisioner " + evidence(claim, detection.EvidenceProvisioner)
	if workload, down := strings.CutSuffix(runs, noReadyPods); down {
		return []sentence{{part: partProof, weight: weightProvisioner,
			text: subject + " runs as " + workload +
				", which has no ready pods."}}
	}
	if runs != provisionerNone {
		return nil
	}
	text := "Claim " + claim.Entity.Name + " is Pending: kwatch sees " +
		"no workload in the cluster serving its provisioner " +
		evidence(claim, detection.EvidenceProvisioner) +
		" (it may run outside the cluster)"
	if said := provisioningError(f); said != "" {
		text += "; the last provisioning failure says " + quoted(said)
	}
	return []sentence{{part: partProof, weight: weightProvisioner,
		text: text + "."}}
}

// provisioningError is the message of the claim's ProvisioningFailed
// event, quoted as the cluster wrote it.
func provisioningError(f caseFacts) string {
	for _, m := range f.members {
		if m.Reason == reasons.ProvisioningFailed {
			if said := evidence(m, "event"); said != "" {
				return said
			}
		}
	}
	return ""
}
