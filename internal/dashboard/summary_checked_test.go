package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func checkedIncident(checked ...string) incident.Incident {
	p := testIncident("c1", "shop", 0)
	p.Root.Kind = "deployment"
	p.Checked = checked
	return p
}

func TestSummaryListsWhatWasCheckedWithoutACause(t *testing.T) {
	p := checkedIncident("node", "configmap", "secret")
	p.Evidence = []incident.Section{{Check: incident.CheckLogs,
		Status: incident.SectionDenied, Reason: incident.ReasonRBAC}}

	d := buildDetail(p, detailContext{now: testEpoch})

	require.NotNil(t, d.Summary)
	assert.Equal(t, &SummaryChecked{
		Fine: []string{"its node is healthy",
			"its ConfigMaps and Secrets are there and did not change"},
		Unchecked: []string{"kwatch cannot read pod logs here"},
	}, d.Summary.Why.Checked)
}

func TestSummaryCheckedAbsentWhenNothingWasChecked(t *testing.T) {
	d := buildDetail(checkedIncident(), detailContext{now: testEpoch})

	require.NotNil(t, d.Summary)
	assert.Nil(t, d.Summary.Why.Checked)
}

func TestSummaryCheckedAlsoForAPossibleCause(t *testing.T) {
	p := checkedIncident("image")
	p.Cause = &rootcause.CauseRecord{Root: p.Root, Score: 0.9,
		Rival: &rootcause.CauseRecord{Score: 0.89}}

	d := buildDetail(p, detailContext{now: testEpoch})

	assert.Equal(t, &SummaryChecked{Fine: []string{"its image"},
		Also: true}, d.Summary.Why.Checked)
}

func TestSummaryCheckedNamesLostLogAccess(t *testing.T) {
	d := buildDetail(checkedIncident(),
		detailContext{now: testEpoch, logsForbidden: true})

	assert.Equal(t, &SummaryChecked{
		Unchecked: []string{"kwatch cannot read pod logs here"}},
		d.Summary.Why.Checked)
}

func TestSummaryCheckedNothingForAConfidentCause(t *testing.T) {
	p := checkedIncident("node")
	p.Cause = &rootcause.CauseRecord{Root: p.Root, Score: 0.95}

	d := buildDetail(p, detailContext{now: testEpoch, logsForbidden: true})

	assert.Nil(t, d.Summary.Why.Checked)
}
