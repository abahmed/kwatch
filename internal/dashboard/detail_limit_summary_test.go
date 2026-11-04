package dashboard

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// worstSummary is a summary with every part far past what is built.
func worstSummary() *IncidentSummary {
	pad := strings.Repeat("z", 4000)
	s := &IncidentSummary{What: pad, Why: SummaryWhy{Text: pad},
		Next: &SummaryNext{Text: pad, Command: "kubectl " + pad[:2000]}}
	for i := range 40 {
		s.Proof = append(s.Proof, SummaryProof{Kind: "log", Text: pad,
			Detail: pad})
		s.Told = append(s.Told, SummaryTold{Name: pad[:300],
			Provider: fmt.Sprint("p", i), Kind: channelOther,
			State: "sent"})
	}
	return s
}

func TestFitDetailFitsTheWorstCaseWithASummary(t *testing.T) {
	d := worstDetail()
	d.Summary = worstSummary()

	fitDetail(&d, detailBudget)

	if n := detailSize(&d); n > detailBudget {
		t.Fatalf("detail is %d bytes, over %d", n, detailBudget)
	}
	require.NotNil(t, d.Truncated)
}

func TestFitDetailCutsTheSummaryLast(t *testing.T) {
	d := worstDetail()
	d.Summary = &IncidentSummary{What: "Deployment api keeps crashing.",
		Proof: []SummaryProof{{Kind: "log", Detail: "panic"}},
		Next:  &SummaryNext{Command: "kubectl logs deploy/api"},
		Told:  []SummaryTold{{Name: "Slack", State: "sent"}}}
	want := *d.Summary

	fitDetail(&d, detailBudget)

	require.NotNil(t, d.Summary)
	assert.Equal(t, want, *d.Summary)
	assert.Zero(t, d.Truncated.SummaryProof+d.Truncated.SummaryTold+
		d.Truncated.SummaryNext)
}

func TestFitDetailDropsSummaryPartsWhenNothingElseIsLeft(t *testing.T) {
	empty := IncidentDetail{Summary: &IncidentSummary{}}
	d := IncidentDetail{Summary: worstSummary()}

	fitDetail(&d, detailSize(&empty)+120)

	require.NotNil(t, d.Truncated)
	assert.Positive(t, d.Truncated.SummaryTold)
	assert.Positive(t, d.Truncated.SummaryProof)
	assert.Equal(t, 1, d.Truncated.SummaryNext)
	assert.Empty(t, d.Summary.Told)
	assert.Empty(t, d.Summary.Proof)
	assert.Nil(t, d.Summary.Next)
	assert.LessOrEqual(t, len([]rune(d.Summary.What)), 40)
}
