package compose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var ongoingNow = time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)

// reasonCodes are every reason code of reasons.go, read from its source
// so a new one is covered without touching this test.
func reasonCodes(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(),
		"../../detection/reasons/reasons.go", nil, 0)
	require.NoError(t, err)
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for _, v := range spec.Values {
			lit, ok := v.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			if code, err := strconv.Unquote(lit.Value); err == nil &&
				len(code) > 5 && code != strings.ToLower(code) {
				out = append(out, code)
			}
		}
		return true
	})
	require.NotEmpty(t, out)
	return out
}

func ongoingIncident(reason string, since time.Time) incident.Incident {
	root := inventory.CoreID(kube.KindDeployment, "shop", "web")
	finding := detection.Finding{Entity: root, Reason: reason,
		Severity: detection.Info, Since: since}
	return incident.Incident{ID: "inc-1", Root: root, Tier: incident.Digest,
		State: incident.Open, Opened: since, Members: members(finding)}
}

// The ongoing line is the incident's own reminder wording: one phrasing
// for one problem, and never a raw reason code.
func TestOngoingTitleSaysTheReminderWordsNoReasonCode(t *testing.T) {
	since := ongoingNow.Add(-4 * 24 * time.Hour)
	for _, code := range reasonCodes(t) {
		title := Writer{}.OngoingTitle(ongoingIncident(code, since),
			ongoingNow)

		assert.Contains(t, title, "still failing, for four days now",
			code)
		assert.NotContains(t, title, code, "raw reason code in the line")
	}
}

func TestStartedAtIsTheEarlierRealTime(t *testing.T) {
	opened := time.Date(2026, 10, 6, 17, 12, 0, 0, time.UTC)
	deleting := time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC)

	p := ongoingIncident(reasons.StuckDeleting, deleting)
	p.Opened = opened
	assert.Equal(t, deleting, StartedAt(p), "the finding began first")

	p = ongoingIncident(reasons.StuckDeleting, time.Time{})
	p.Opened = opened
	assert.Equal(t, opened, StartedAt(p), "a finding without a time")

	p = ongoingIncident(reasons.StuckDeleting, opened.Add(time.Hour))
	p.Opened = opened
	assert.Equal(t, opened, StartedAt(p), "the opening began first")
}

// A node that was under pressure was never NotReady: its resolve names
// what ended.
func TestNodeResolveSaysTheFailureThatEnded(t *testing.T) {
	node := inventory.CoreID(kube.KindNode, "", "ip-10-0-65-81")
	for _, c := range []struct {
		finding detection.Finding
		want    string
	}{
		{detection.Finding{Reason: reasons.NodeNotReady},
			"is ready again"},
		{detection.Finding{Reason: reasons.NodePSIHigh,
			Summary: "Node is under CPU pressure: workloads stall on CPU"},
			"is no longer under CPU pressure"},
		{detection.Finding{Reason: reasons.MemoryPressure},
			"is no longer under memory pressure"},
		{detection.Finding{Reason: reasons.NodeFilesystemHigh},
			"is no longer low on disk space"},
	} {
		c.finding.Entity = node
		p := incident.Incident{ID: "inc-n", Root: node,
			State: incident.Resolved, Opened: at(0, 0), Resolved: at(9, 0),
			Members: members(c.finding)}

		_, got := resolveNote(caseFacts{p: p, members: sortedMembers(p),
			now: at(10, 0), reason: "healthy"})

		assert.Contains(t, got[0].text, c.want, c.finding.Reason)
	}
}
