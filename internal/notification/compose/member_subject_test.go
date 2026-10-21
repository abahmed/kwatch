package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// memberLeadCase is an incident whose lead is not the container that
// carries the facts: a node (or Secret) is the lead, and a container
// that runs on it holds the numbers.
func memberLeadCase(
	root inventory.EntityID, lead detection.Finding,
	crash detection.Finding,
) incident.Incident {
	return incident.Incident{
		ID: "inc-1", Root: root, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(lead, crash),
		Impact: []inventory.EntityID{
			inventory.CoreID(kube.KindDeployment, "shop", "api")},
	}
}

func apiContainer() inventory.EntityID {
	return inventory.CoreID(kube.KindContainer, "shop", "api-7d9f-a/app")
}

func TestNodeOOMNamesTheContainerUnderANodeLead(t *testing.T) {
	node := inventory.CoreID(kube.KindNode, "", "n1")
	p := memberLeadCase(node,
		detection.Finding{Entity: node, Reason: reasons.MemoryPressure,
			Severity: detection.Critical, Since: writerNow,
			Summary: "Node is under memory pressure"},
		detection.Finding{Entity: apiContainer(),
			Reason: reasons.OOMKilled, Severity: detection.Critical,
			Since: writerNow, Summary: "Container was OOM-killed",
			Evidence: []detection.Evidence{
				{Label: detection.EvidenceKilledByNode, Value: "n1"},
				{Label: detection.EvidenceMemoryUsed, Value: "180Mi"},
				{Label: detection.EvidenceMemoryLimit, Value: "512Mi"},
			}})

	got := Writer{}.Write(announce(p), writerNow).Note

	assert.Contains(t, got, "Container app in pod api-7d9f-a in shop "+
		"used 180Mi of its 512Mi limit.")
	assert.NotContains(t, got, "It used 180Mi")
}

func TestNodeOOMKeepsItWhenTheContainerIsTheLead(t *testing.T) {
	got := Writer{}.Write(announce(nodeOOMKill()), writerNow).Note

	assert.Contains(t, got, "It used 180Mi of its 512Mi limit.")
}

func TestHowFailedNamesTheContainerUnderASecretLead(t *testing.T) {
	secret := inventory.CoreID(kube.KindSecret, "shop", "bank-tls")
	p := memberLeadCase(secret,
		detection.Finding{Entity: secret, Reason: reasons.TLSCertExpired,
			Severity: detection.Critical, Since: writerNow,
			Summary: "TLS certificate expired one hour ago"},
		detection.Finding{Entity: apiContainer(),
			Reason: reasons.CrashLoopBackOff, Severity: detection.Critical,
			Since: writerNow, Summary: "Container is crash looping",
			Evidence: []detection.Evidence{
				{Label: detection.EvidenceRanFor, Value: "10s"},
				{Label: detection.EvidenceRestartSpan, Value: "1h4m"},
				{Label: "restarts", Value: "2"},
			}})

	got := Writer{}.Write(announce(p), writerNow).Note

	assert.Contains(t, got, "The last run of container app in pod "+
		"api-7d9f-a lasted ten seconds")
	assert.NotContains(t, got, "Its last run")
}

func TestHowFailedKeepsItForTheLeadsOwnContainer(t *testing.T) {
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	p := memberLeadCase(api,
		detection.Finding{Entity: api, Reason: reasons.CrashLoopBackOff,
			Severity: detection.Critical, Since: writerNow,
			Symptom: true, Summary: "Workload is below its replicas"},
		detection.Finding{Entity: apiContainer(),
			Reason: reasons.CrashLoopBackOff, Severity: detection.Critical,
			Since: writerNow, Summary: "Container is crash looping",
			Evidence: []detection.Evidence{
				{Label: detection.EvidenceRanFor, Value: "10s"},
			}})

	got := Writer{}.Write(announce(p), writerNow).Note

	assert.Contains(t, got, "Its last run lasted ten seconds.")
}
