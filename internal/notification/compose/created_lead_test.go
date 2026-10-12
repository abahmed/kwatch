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
	"github.com/abahmed/kwatch/internal/rootcause"
)

// createdWorkload is a workload whose own creation is the blamed change.
func createdWorkload(
	kind inventory.Kind, reason string, failingAfter time.Duration,
) incident.Incident {
	id := inventory.CoreID(kind, "shop", "api")
	created := writerNow.Add(-10 * time.Minute)
	return incident.Incident{
		ID: "created-1", Root: id, Tier: incident.Notify,
		State: incident.Open, Opened: created.Add(failingAfter),
		Members: members(detection.Finding{Entity: id, Reason: reason,
			Severity: detection.Warning, Since: created.Add(failingAfter),
			Summary: "0 of 1 replicas are ready"}),
		Cause: &rootcause.CauseRecord{Root: id, Score: 0.9,
			Change: &inventory.Change{Entity: id, Created: true,
				At: created},
			Proof: []rootcause.Proof{{
				Code: rootcause.ProofFailedAfterChange, Supports: true,
				Weight: 0.3}}},
	}
}

func TestWriteSaysAWorkloadNeverWorkedSinceItWasCreated(t *testing.T) {
	tests := []struct {
		name   string
		kind   inventory.Kind
		reason string
	}{
		{"deployment", kube.KindDeployment, reasons.DeploymentUnavailable},
		{"stateful set", kube.KindStatefulSet, reasons.DeploymentUnavailable},
		{"daemon set", kube.KindDaemonSet, reasons.DeploymentUnavailable},
		{"crash loop", kube.KindDeployment, reasons.CrashLoopBackOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := createdWorkload(tt.kind, tt.reason, 30*time.Second)

			msg := Writer{}.Write(announce(p), writerNow)

			assert.Contains(t, msg.Note, "api in shop has never worked "+
				"since it was created at 09:50.")
			assert.NotContains(t, msg.Note, "release")
			assert.NotContains(t, msg.Note, "soon after it changed")
		})
	}
}

// A workload that ran for a while before it failed did work.
func TestWriteDoesNotSayNeverWorkedForALateFailure(t *testing.T) {
	p := createdWorkload(kube.KindDeployment,
		reasons.DeploymentUnavailable, 8*time.Minute)

	msg := Writer{}.Write(announce(p), writerNow)

	assert.NotContains(t, msg.Note, "never worked")
	assert.Contains(t, msg.Note, "after the 09:50 creation")
	assert.NotContains(t, msg.Note, "release")
	assert.NotContains(t, msg.Note, "soon after it changed")
}

// A created policy that broke a workload still failed it after it changed.
func TestWriteKeepsFailedAfterChangeForACreatedNonWorkload(t *testing.T) {
	p := createdWorkload(kube.KindDeployment,
		reasons.CrashLoopBackOff, 30*time.Second)
	policy := inventory.CoreID(kube.KindNetworkPolicy, "shop", "deny-all")
	p.Cause.Root = policy
	p.Cause.Change = &inventory.Change{Entity: policy, Created: true,
		At: writerNow.Add(-10 * time.Minute)}

	msg := Writer{}.Write(announce(p), writerNow)

	assert.Contains(t, msg.Note, "dependents failed soon after it changed")
}
