package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// configSplitIncident is an api Deployment failing because pods started
// after its ConfigMap changed crash while the older ones run.
func configSplitIncident(proof rootcause.Proof) incident.Incident {
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	cfg := inventory.CoreID(kube.KindConfigMap, "shop", "api-config")
	ct := inventory.CoreID(kube.KindContainer, "shop", "api-4d/app")
	proof.Supports, proof.Weight = true, 0.15
	return incident.Incident{
		ID: "inc-8", Root: cfg, Tier: incident.Notify, State: incident.Open,
		Opened: at(3, 0), Revision: 1,
		Members: members(detection.Finding{Entity: ct,
			Reason: reasons.CrashLoopBackOff, Severity: detection.Critical,
			Since: at(3, 0), Summary: "Container is crash looping"}),
		Cause: &rootcause.CauseRecord{Root: cfg, Score: 0.6,
			Chain: []inventory.EntityID{cfg, api, ct},
			Change: &inventory.Change{Entity: cfg, At: at(0, 0),
				Actor: "alice", Fields: []inventory.FieldChange{
					{Path: "data.DB_HOST", After: "changed"},
					{Path: "data.TIMEOUT", After: "changed"}}},
			Summary: "configmap api-config changed",
			Proof:   []rootcause.Proof{proof}},
	}
}

func TestConfigVersionSaysWhichPodsFail(t *testing.T) {
	p := configSplitIncident(rootcause.Proof{
		Code: rootcause.ProofNewConfigFails, Count: 2, Total: 3})

	text := notification.Text(Writer{}.Write(announce(p), at(10, 0)))

	assert.Contains(t, text, "Config map api-config changed ten minutes "+
		"ago (keys: DB_HOST, TIMEOUT; by alice); the two pods started "+
		"since then fail, the three older ones are healthy.")
	assert.NotContains(t, text, "bob")
}

func TestConfigVersionOldPodsFail(t *testing.T) {
	p := configSplitIncident(rootcause.Proof{
		Code: rootcause.ProofOldConfigFails, Count: 3, Total: 1})

	text := notification.Text(Writer{}.Write(announce(p), at(10, 0)))

	assert.Contains(t, text, "the three pods still on the old version "+
		"fail, the one started since then is healthy.")
}

func TestConfigVersionStaysQuietWithoutTheProof(t *testing.T) {
	p := configSplitIncident(rootcause.Proof{Text: "it changed"})

	text := notification.Text(Writer{}.Write(announce(p), at(10, 0)))

	assert.NotContains(t, text, "started since then")
}

func writeConfigVersionSplit() notification.Message {
	p := configSplitIncident(rootcause.Proof{
		Code: rootcause.ProofNewConfigFails, Count: 2, Total: 3})
	return Writer{}.Write(announce(p), at(10, 0))
}
