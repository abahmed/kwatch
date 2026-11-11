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
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// stuckRelease is api in shop after a release whose pods stop at start
// with line; serves says the previous version still answers.
func stuckRelease(
	line string, serves bool, tier incident.Tier,
) incident.Incident {
	deploy := inventory.CoreID(kube.KindDeployment, "shop", "api")
	pod := inventory.CoreID(kube.KindPod, "shop", "api-2-x")
	release := inventory.Change{Entity: deploy,
		At: writerNow.Add(-20 * time.Minute),
		Fields: []inventory.FieldChange{{
			Path:  "spec.template.spec.containers[0].image",
			After: "registry.example/team/api:2.3"}}}
	return incident.Incident{
		ID: "start-1", Root: deploy, Tier: tier, State: incident.Open,
		Opened: writerNow.Add(-15 * time.Minute), PreviousServes: serves,
		Cause: &rootcause.CauseRecord{Rule: "rollout",
			Mode: explain.ModeSpecChanged, Root: deploy,
			Chain: []inventory.EntityID{deploy}, Change: &release,
			SharedError: line},
		Members: members(detection.Finding{Entity: pod,
			Reason: reasons.CrashLoopBackOff, Severity: detection.Critical,
			Since:   writerNow.Add(-15 * time.Minute),
			Summary: "Container app keeps crashing"}),
	}
}

const startLine = "Unhandled exception. Config.Error: setting " +
	"'Store' is required."

// The fatal line is the cause, the release names the version, and the
// previous version still serving is said instead of "is down".
func TestStartLeadNamesTheVersionTheErrorAndWhatServes(t *testing.T) {
	p := stuckRelease(startLine, true, incident.Notify)
	msg := Writer{}.Write(announce(p), writerNow)
	assert.Contains(t, msg.Title, "api in shop: the new version "+
		"(api:2.3) can't start (it stops at start with")
	assert.Contains(t, msg.Title, "); the previous version is still "+
		"serving")
	assert.Contains(t, msg.Title, "setting 'Store' is required")
	assert.NotContains(t, msg.Title, "is down")
	assert.NotContains(t, msg.Title, "too little time")
}

// Nothing serves: the usual rollout lead, which blames the release.
func TestStartLeadNotWhenNothingServes(t *testing.T) {
	p := stuckRelease(startLine, false, incident.Page)
	msg := Writer{}.Write(announce(p), writerNow)
	assert.NotContains(t, msg.Title, "previous version is still serving")
	assert.Contains(t, msg.Title, "release of registry.example/team/api:2.3")
}

// "exec format error" is a binary built for another CPU.
func TestStartLeadNamesAWrongArchitecture(t *testing.T) {
	p := stuckRelease("exec /usr/bin/sh: exec format error", true,
		incident.Notify)
	for k, m := range p.Members {
		m.Evidence = append(m.Evidence, detection.Evidence{
			Label: detection.EvidenceArchNode, Value: "arm64 node n1"})
		p.Members[k] = m
	}
	msg := Writer{}.Write(announce(p), writerNow)
	assert.Contains(t, msg.Title, "is not built for this node's CPU "+
		"(arm64)")
}

// Without a fatal line, a rollout that cannot start while the previous
// version serves says so, not "is failing" or "is down".
func TestStartLeadForAStuckRollout(t *testing.T) {
	p := stuckRelease("", true, incident.Notify)
	p.Cause = &rootcause.CauseRecord{Rule: "self", Root: p.Root}
	msg := Writer{}.Write(announce(p), writerNow)
	assert.Contains(t, msg.Title, "api in shop: the new version keeps "+
		"failing; the previous version is still serving")
}

// The Short drops the quoted start error, so a provider that cuts it
// short still says the previous version serves; the CPU stays.
func TestStartLeadShortDropsTheQuote(t *testing.T) {
	p := stuckRelease(startLine, true, incident.Notify)
	msg := Writer{}.Write(announce(p), writerNow)
	assert.Contains(t, msg.Title, "setting 'Store' is required")
	assert.Equal(t, notification.MarkerNotify+" api in shop: the new "+
		"version (api:2.3) can't start; the previous version is still "+
		"serving.", msg.Short)

	arch := stuckRelease("exec /usr/bin/sh: exec format error", true,
		incident.Notify)
	for k, m := range arch.Members {
		m.Evidence = append(m.Evidence, detection.Evidence{
			Label: detection.EvidenceArchNode, Value: "arm64 node n1"})
		arch.Members[k] = m
	}
	short := Writer{}.Write(announce(arch), writerNow).Short
	assert.Contains(t, short, "can't start (it is not built for this "+
		"node's CPU (arm64)); the previous version")
	assert.NotContains(t, short, "exec format error")
}
