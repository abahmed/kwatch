package compose

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// The problem title is the announcement's lead whatever the incident's
// state: a resolved incident is not titled as a recovery, nor a
// settling one left untitled. The cluster tag is left to the page.
func TestProblemTitleIsTheLeadOfTheAnnouncement(t *testing.T) {
	p := crashIncident()
	w := NewWriter("prod-eu-1", nil)
	open := w.ProblemTitle(p, nil, revisionNow)

	assert.NotEmpty(t, open)
	assert.NotContains(t, open, "prod-eu-1")
	for _, state := range []incident.State{incident.Settling,
		incident.Recovering, incident.Resolved} {
		p.State = state
		assert.Equal(t, open, w.ProblemTitle(p, nil, revisionNow), state)
	}
	p.Members = nil
	assert.Empty(t, w.ProblemTitle(p, nil, revisionNow))
}

// The Community title says what fails and never names the cause's
// object; that, as a category, is for the message.
func TestProblemTitleOfCommunityNamesNoCause(t *testing.T) {
	p := crashIncident()
	p.Cause = &rootcause.CauseRecord{Summary: "ConfigMap settings changed",
		Root: inventory.CoreID("configmap", "shop", "settings"),
		Rule: "configmap-missing-or-changed"}
	w := Writer{Community: true}

	got := w.ProblemTitle(p, nil, revisionNow)

	assert.NotEmpty(t, got)
	assert.NotContains(t, got, "settings")
}

// A long lead is clipped on a word, within the bound.
func TestProblemTitleIsBounded(t *testing.T) {
	long := strings.Repeat("word ", 60)
	got := clipTitle(long)

	assert.LessOrEqual(t, utf8.RuneCountInString(got), maxProblemTitle)
	assert.True(t, strings.HasSuffix(got, "word…"), got)
	assert.Equal(t, "short.", clipTitle("short."))
}

// An incident moved into a shared-error incident says so in words, not
// by the normalised name of the signature.
func TestMovedIntoASharedErrorSaysSoInWords(t *testing.T) {
	p := crashIncident()
	p.Members = nil
	p.State, p.Resolved = incident.Resolved, revisionNow
	p.SupersededBy = "inc-2"
	p.SupersededRoot = inventory.CoreID(explain.KindFailureSignature, "",
		"crashloop scheduling did not satisfy plugin")
	d := incident.Decision{Action: incident.Resolve, Incident: p,
		Reason: incident.ReasonSuperseded}

	msg := Writer{}.Write(d, revisionNow)

	assert.Contains(t, msg.Title, "Moved: this is now part of the "+
		"incident of workloads failing with the same error")
	assert.NotContains(t, msg.Title, "plugin")
}

// A flapping incident is titled as its alert leads: a workload out of
// service says so, not the lead of a plain open incident.
func TestProblemTitleOfFlappingMatchesTheAlert(t *testing.T) {
	p := flappingSearch()
	p.Members = members(readinessMember("Readiness probe failed: "+
		"HTTP probe failed with statuscode: 503", false))
	w := Writer{}

	got := w.ProblemTitle(p, nil, at(41, 0))
	alert := w.Write(announce(p), at(41, 0)).Title

	assert.Contains(t, got, "is not serving: its readiness check fails")
	assert.Equal(t, clipTitle(alert), got)
}
