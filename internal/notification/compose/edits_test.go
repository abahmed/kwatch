package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func TestEditWordsReadsLikeAPersonWouldSayIt(t *testing.T) {
	tests := []struct {
		name  string
		field inventory.FieldChange
		want  string
	}{
		{"env", inventory.FieldChange{
			Path: "containers[app].env.DB_HOST", Before: "db-old",
			After: "db-new"}, "env DB_HOST db-old → db-new"},
		{"memory limit", inventory.FieldChange{
			Path:   "containers[app].resources.limits.memory",
			Before: "512Mi", After: "256Mi"},
			"memory limit 512Mi → 256Mi"},
		{"added env", inventory.FieldChange{
			Path: "containers[app].env.MODE", After: "fast"},
			"env MODE unset → fast"},
		{"credential env", inventory.FieldChange{
			Path:   "containers[app].env.DB_PASSWORD",
			Before: "changed", After: "changed"},
			"env DB_PASSWORD changed"},
		{"probe", inventory.FieldChange{
			Path:   "containers[app].livenessProbe",
			Before: "http /a every 5s", After: "http /b every 5s"},
			"liveness probe http /a every 5s → http /b every 5s"},
		{"volume", inventory.FieldChange{Path: "volumes[cfg]",
			Before: "configmap/a", After: "configmap/b"},
			"volume cfg configmap/a → configmap/b"},
		{"image", inventory.FieldChange{
			Path: "containers[app].image", Before: "app:1", After: "app:2"},
			"image app:1 → app:2"},
		{"config key", inventory.FieldChange{Path: "data.LOG",
			After: "changed"}, "key LOG changed"},
		{"container added", inventory.FieldChange{
			Path: "containers[side]", After: "added"}, "container side added"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, editWords(tc.field))
		})
	}
}

func revisionCause(edits ...inventory.FieldChange) *rootcause.CauseRecord {
	cause := badRollout().Cause
	cause.Proof = []rootcause.Proof{{Code: rootcause.ProofNewRevisionFails,
		Count: 14, Edits: edits, Weight: 0.3, Supports: true}}
	return cause
}

// The failing revision names its only change, who made it, and how the
// pods fail; the change sentence it would repeat is not written.
func TestRevisionDiffNamesTheOnlyChange(t *testing.T) {
	p := badRollout()
	p.Cause = revisionCause(inventory.FieldChange{
		Path:   "containers[app].resources.limits.memory",
		Before: "512Mi", After: "256Mi"})
	p.Cause.Change.Fields = []inventory.FieldChange{{
		Path:   "containers[app].resources.limits.memory",
		Before: "512Mi", After: "256Mi"}}
	for key, m := range p.Members {
		m.Reason = reasons.OOMKilled
		p.Members[key] = m
	}

	text := notification.Text(Writer{}.Write(announce(p), at(6, 0)))

	want := "Only change in revision 14: memory limit 512Mi → 256Mi " +
		"(by alice); pods OOMKilled since."
	assert.Contains(t, text, want)
	assert.NotContains(t, text, "release changed")
}

func TestRevisionDiffRanksSeveralEdits(t *testing.T) {
	p := badRollout()
	p.Cause = revisionCause(
		inventory.FieldChange{Path: "containers[app].image",
			Before: "app:1", After: "app:2"},
		inventory.FieldChange{Path: "containers[app].env.A",
			Before: "1", After: "2"})

	text := notification.Text(Writer{}.Write(announce(p), at(6, 0)))

	assert.Contains(t, text, "Changes in revision 14, likeliest culprit "+
		"first: image app:1 → app:2; env A 1 → 2")
}

// A release regression says what its revision changed from its own
// evidence.
func TestReleaseRegressionSaysWhatTheRevisionChanged(t *testing.T) {
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	p := badRollout()
	p.Root, p.Cause = api, nil
	p.Members = members(detection.Finding{Entity: api,
		Reason: reasons.ReleaseRegression, Severity: detection.Warning,
		Mode:  detection.ModeReleaseRegression,
		Since: at(2, 0), Summary: "Rollout 14 restarted 7 times",
		Evidence: []detection.Evidence{
			{Label: "new revision", Value: "14"},
			{Label: detection.EvidenceEditPrefix + "containers[app].args",
				Value: "unset" + detection.EvidenceEditArrow + "--fast"}}})

	text := notification.Text(Writer{}.Write(announce(p), at(6, 0)))

	assert.Contains(t, text, "Only change in revision 14: args unset → "+
		"--fast; pods restarting more than before since.")
}

// With a cause, the neighbours that changed before the failure are named
// after the proof, each with what it edited.
func TestNearbyChangesAreNamedNextToACause(t *testing.T) {
	cm := inventory.CoreID(kube.KindConfigMap, "shop", "app-config")
	svc := inventory.CoreID(kube.KindService, "shop", "payments")
	d := announce(badRollout())
	d.Facts.Changes = []inventory.Change{
		{Entity: payments, Actor: "alice", At: at(1, 0),
			Fields: []inventory.FieldChange{{
				Path: "containers[app].env.DB_HOST", Before: "db-old",
				After: "db-new"}}},
		changeAt(cm, "bob", 1, "data.LOG"),
		{Entity: svc, At: at(1, 30), Fields: []inventory.FieldChange{{
			Path: "spec.selector", Before: "app=a", After: "app=b"}}},
	}

	text := notification.Text(Writer{}.Write(d, at(6, 0)))

	want := "Also changed before it broke: payments: env DB_HOST db-old " +
		"→ db-new at 14:01 by alice; bob changed config map app-config " +
		"at 14:01; service payments: selector app=a → app=b at 14:01."
	assert.Contains(t, text, want)
}
