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

var sameCheckEvidence = detection.Evidence{
	Label: detection.EvidenceLivenessSameCheck, Value: "true"}

// cascadeIncident is a db Deployment that fails, with liveness kills of
// api and worker pods among its members.
func cascadeIncident(check ...detection.Evidence) incident.Incident {
	db := inventory.CoreID(kube.KindDeployment, "shop", "db")
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	worker := inventory.CoreID(kube.KindDeployment, "shop", "worker")
	var findings []detection.Finding
	findings = append(findings, detection.Finding{Entity: db,
		Reason: reasons.DeploymentUnavailable, Severity: detection.Critical,
		Since: at(2, 0), Summary: "Deployment has no ready replicas"})
	for _, name := range []string{"api-4d-a", "api-4d-b", "worker-9c-a"} {
		findings = append(findings, detection.Finding{
			Entity: inventory.CoreID(kube.KindContainer, "shop",
				name+"/app"),
			Reason: reasons.LivenessKilled, Severity: detection.Critical,
			Since:    at(3, 0),
			Summary:  "Container keeps being killed by its liveness probe",
			Evidence: check})
	}
	return incident.Incident{ID: "inc-c", Root: db, Tier: incident.Notify,
		State: incident.Open, Opened: at(2, 0),
		Impact:  []inventory.EntityID{api, worker},
		Members: members(findings...),
		Cause: &rootcause.CauseRecord{Root: db, Score: 0.9,
			Rule: "called-service-backends-failing"}}
}

func TestLivenessCascadeSaysTheRestartsAreASymptom(t *testing.T) {
	p := cascadeIncident(sameCheckEvidence)

	text := Writer{}.Write(announce(p), at(10, 0)).Note

	assert.Contains(t, text, "db went down at 14:02; 3 pods of api and "+
		"worker were then restarted by their liveness probe (same "+
		"check as readiness), so the restarts are a symptom.")
}

func TestLivenessCascadeNeedsTheSameCheck(t *testing.T) {
	p := cascadeIncident()

	text := Writer{}.Write(announce(p), at(10, 0)).Note

	assert.NotContains(t, text, "same check as readiness")
}
