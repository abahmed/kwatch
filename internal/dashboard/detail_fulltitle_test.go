package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The list item keeps the short title; the detail also carries the
// whole one for the page's heading.
func TestDetailCarriesTheFullTitle(t *testing.T) {
	p := summaryIncident()
	p.Problem = "api in shop can't start: it stops at start with " +
		"\"Unhandled…"
	p.ProblemFull = "api in shop can't start: it stops at start with " +
		"\"Unhandled exception. Bad config\""

	d := toDetail(p)

	assert.Equal(t, p.Problem, d.Title)
	assert.Equal(t, p.ProblemFull, d.FullTitle)
	assert.Equal(t, p.Problem, toListItem(p).Title)

	p.ProblemFull = ""
	assert.Equal(t, p.Problem, toDetail(p).FullTitle,
		"an older record has the short one only")
}
