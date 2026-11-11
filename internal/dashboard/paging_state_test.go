package dashboard

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
)

// pagingIncidents is a reader that also says how paging stands, as
// *incident.Manager does.
type pagingIncidents struct {
	fakeIncidents
	scaling incident.ScalingState
	nonProd bool
}

func (p *pagingIncidents) Scaling(time.Time) incident.ScalingState {
	return p.scaling
}

func (p *pagingIncidents) NonProduction() bool { return p.nonProd }

func readOverview(t *testing.T, s *Server) (OverviewResponse, string) {
	t.Helper()
	w := get(t, s, "/api/overview")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got OverviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	return got, w.Body.String()
}

// While the cluster scales the Overview says pages are held, since when
// and until when.
func TestOverviewSaysPagesAreHeldWhileTheClusterScales(t *testing.T) {
	s := newTestServer()
	end := testEpoch.Add(10 * time.Minute)
	s.SetIncidents(&pagingIncidents{scaling: incident.ScalingState{
		Held: true, Since: testEpoch, Expected: end}})

	got, _ := readOverview(t, s)

	require.NotNil(t, got.ScalingHold)
	assert.Equal(t, ScalingHold{Reason: "Paging held: cluster is scaling",
		Since: testEpoch, ExpectedEnd: end}, *got.ScalingHold)
}

func TestOverviewWithoutScalingHasNoHold(t *testing.T) {
	s := newTestServer()
	_, body := readOverview(t, s)
	assert.NotContains(t, body, "scalingHold")

	s.SetIncidents(&pagingIncidents{})
	_, body = readOverview(t, s)
	assert.NotContains(t, body, "scalingHold")
}

// A non-production cluster pages nobody, so the Overview does not say
// a page is held while it scales.
func TestOverviewHasNoHoldOnANonProductionCluster(t *testing.T) {
	s := newTestServer()
	s.SetIncidents(&pagingIncidents{nonProd: true,
		scaling: incident.ScalingState{Held: true, Since: testEpoch,
			Expected: testEpoch.Add(10 * time.Minute)}})

	got, body := readOverview(t, s)

	assert.Nil(t, got.ScalingHold)
	assert.NotContains(t, body, "scalingHold")
}

// kwatch's own page says whether the cluster is set as non-production.
func TestKwatchStatusSaysNonProduction(t *testing.T) {
	s := newTestServer()
	got, body := readKwatch(t, s)
	assert.False(t, got.NonProduction)
	assert.Contains(t, body, `"nonProduction":false`)

	s.SetIncidents(&pagingIncidents{nonProd: true})
	got, _ = readKwatch(t, s)
	assert.True(t, got.NonProduction)
}

// A held page says why it waits; a capped one why it did not page.
func TestIncidentWhySaysPagingHeldAndNonProduction(t *testing.T) {
	p := testIncident("a", "shop", 0)
	p.State = incident.Settling
	p.Journal.Timers = incident.Timers{At: testEpoch, ScalingHeld: true}
	why := whyOf(p, testEpoch)
	require.NotNil(t, why)
	assert.Equal(t, "Paging held: cluster is scaling", why.PagingHeldReason)

	p = testIncident("b", "shop", 0)
	p.Journal.Level = "non-production"
	why = whyOf(p, testEpoch)
	require.NotNil(t, why)
	assert.Contains(t, why.AlertLevelReason,
		"Not paged: non-production cluster")
	assert.Empty(t, why.PagingHeldReason)
}
