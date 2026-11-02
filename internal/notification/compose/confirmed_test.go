package compose

import (
	"strings"
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

// clippedProof is a crash proof investigation cut before the closing
// quote of the log line it quotes.
const clippedProof = "the last run of api-1/app exited with code 2 " +
	"after logging \"panic: Cannot connect to redis: dial tcp: lookup " +
	"redis.example on…"

// nodeIPDecision is a node out of pod IPs whose member pod crashed;
// investigation confirmed the crash of that pod, and the node check
// read nodeStatus.
func nodeIPDecision(nodeStatus string) incident.Decision {
	root := inventory.CoreID(kube.KindNode, "", "n1")
	crash := detection.Finding{
		Entity: inventory.CoreID(kube.KindContainer, "shop", "api-1/app"),
		Reason: reasons.CrashLoopBackOff, Mode: "CrashLoop",
		Severity: detection.Critical, Since: writerNow.Add(-time.Hour)}
	p := incident.Incident{ID: "ip-c", Root: root, Tier: incident.Page,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(ipNode("n1", "5"), crash),
		Cause: &rootcause.CauseRecord{Root: root, Score: rootcause.High,
			Summary: "node n1 explains 2 failures"}}
	return incident.Decision{Action: incident.Announce, Incident: p,
		Facts: incident.Facts{Report: []incident.Section{
			{Check: incident.CheckLogs, Status: incident.SectionOK},
			{Check: incident.CheckNode, Status: nodeStatus}},
			Correlation: &incident.Correlation{Confirmed: clippedProof}}}
}

func TestConfirmedTextClosesAClippedQuote(t *testing.T) {
	got := plainQuotes(confirmedText(clippedProof))

	assert.True(t, strings.HasSuffix(got, "on…\""), got)
	assert.Equal(t, "plain proof", confirmedText("plain proof"))
	assert.Equal(t, "it logged \"boom\"",
		plainQuotes(confirmedText("it logged \"boom\"")))
}

func TestWriteConfirmedSaysConfirmedAfterTheClosedQuote(t *testing.T) {
	m := NewWriter("", nil).Write(nodeIPDecision(incident.SectionOK),
		writerNow)

	assert.Contains(t, m.Note, "on…\". Confirmed.")
	assert.Equal(t, confirmedWord, m.Confidence)
}

func TestWriteNodeRootSaysItsOwnEffectBeforeAMemberProof(t *testing.T) {
	note := NewWriter("", nil).Write(nodeIPDecision(incident.SectionOK),
		writerNow).Note

	own := strings.Index(note, "5 pods can't start")
	side := strings.Index(note, "The last run of api-1/app")
	assert.True(t, own > 0 && side > own, note)
}

func TestWriteRootUnseenCapsConfirmedAtTheCauseLevel(t *testing.T) {
	d := nodeIPDecision(incident.SectionAbsent)
	m := NewWriter("", nil).Write(d, writerNow)

	assert.Equal(t, "high", m.Confidence)
	assert.NotContains(t, m.Note, "Confirmed.")
	assert.Contains(t, m.Note, "on…\".")
	assert.False(t, m.Evidence.Confirmed)

	d.Incident.Cause.Score = rootcause.Likely
	assert.Equal(t, "likely", NewWriter("", nil).Write(d, writerNow).
		Confidence)
}

func TestWriteWorkloadProofStaysFirstAndConfirmed(t *testing.T) {
	m := NewWriter("", nil).Write(crashDecision(gatheredReport), evidenceAt)

	assert.Equal(t, confirmedWord, m.Confidence)
	assert.False(t, sideProof(gatherFacts(crashDecision(nil),
		evidenceAt, nil)))
}

// A proof about pod api-1 does not name a node called "api": only a
// whole object name counts as naming the root.
func TestSideProofMatchesWholeObjectNames(t *testing.T) {
	d := nodeIPDecision(incident.SectionOK)
	root := inventory.CoreID(kube.KindNode, "", "api")
	d.Incident.Root, d.Incident.Cause.Root = root, root
	assert.True(t, sideProof(gatherFacts(d, evidenceAt, nil)))

	assert.True(t, namesObject("node api ran out of IPs.", "api"))
	assert.True(t, namesObject("api-1/app exited", "api-1/app"))
	assert.False(t, namesObject("api-1/app exited", "api"))
	assert.False(t, namesObject("api.v2 failed", "api"))
	assert.False(t, namesObject("anything", ""))
}
