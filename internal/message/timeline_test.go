package message

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	context "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

func TestBuildTimelineCollapsesRepeatedChanges(t *testing.T) {
	start := time.Date(2025, 1, 1, 16, 56, 0, 0, time.UTC)
	inc := &model.Incident{Status: model.Status{
		FirstSeen: start.Add(30 * time.Second),
		LastSeen:  start.Add(3 * time.Minute),
	}}
	ins := &insight.Insight{}
	for i := 0; i < 3; i++ {
		ins.RecentChanges = append(ins.RecentChanges, context.Change{
			Resource: "horizontalpodautoscaler", Namespace: "staging",
			Name: "transactions", Type: context.ChangeUpdate,
			Timestamp: start.Add(time.Duration(i) * 10 * time.Second),
		})
	}

	out := (&ReportBuilder{}).buildTimeline(inc, ins, model.ActionCreate)

	assert.Equal(t, 1, strings.Count(out, "transactions updated"),
		"three identical entries in one minute read as one")
	assert.Contains(t, out, "transactions updated ×3")
	assert.Contains(t, out, "16:56 started")
	assert.Contains(t, out, "16:59 ongoing")
	assert.True(t, strings.HasSuffix(out, "(UTC)"),
		"the clock is the API server's and must say so")
}

func TestBuildTimelineKeepsDistinctChanges(t *testing.T) {
	start := time.Date(2025, 1, 1, 16, 56, 0, 0, time.UTC)
	inc := &model.Incident{Status: model.Status{
		FirstSeen: start, LastSeen: start.Add(2 * time.Minute),
	}}
	ins := &insight.Insight{RecentChanges: []context.Change{
		{Resource: "deployment", Namespace: "ns", Name: "api",
			Type: context.ChangeUpdate, Timestamp: start.Add(10 * time.Second)},
		{Resource: "configmap", Namespace: "ns", Name: "cfg",
			Type: context.ChangeUpdate, Timestamp: start.Add(20 * time.Second)},
	}}

	out := (&ReportBuilder{}).buildTimeline(inc, ins, model.ActionResolved)

	assert.Contains(t, out, "deployment ns/api updated")
	assert.Contains(t, out, "configmap ns/cfg updated")
	assert.NotContains(t, out, "×")
	assert.Contains(t, out, "resolved")
}

func TestChangeSummaryClipsLongValues(t *testing.T) {
	long := strings.Repeat("x", 300)
	r := &Report{Changes: &ChangesSection{Items: []ChangeItem{{
		Resource: "replicaset", Reference: "ns/pay-56b9", Type: "updated",
		Age: "9s",
		Fields: []FieldChange{{
			Path: "metadata.annotations.some/config", Before: long, After: long,
		}},
	}}}}

	out := ChangeSummary(r)

	assert.Less(t, len(out), 300, "a manifest does not belong in a summary")
	assert.Contains(t, out, "…")
	assert.Contains(t, out, "replicaset ns/pay-56b9 updated 9s ago")
}

func TestNarrativeEvidenceReadsAsASentence(t *testing.T) {
	r := &Report{Diagnosis: &DiagnosisSection{
		Cause:      "the node is under pressure",
		Confidence: 0.8,
		Evidence:   []string{"warning events were observed"},
	}}
	out := Narrative(r)
	assert.Contains(t, out, "Supporting evidence: warning events were observed.")
	assert.NotContains(t, out, "This is supported by")
}
