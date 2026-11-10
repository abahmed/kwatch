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

// A held issue says until when its page is held: the end the Overview
// shows; without a hold it says nothing of it.
func TestIncidentDetailSaysUntilWhenPagingIsHeld(t *testing.T) {
	p := testIncident("a", "shop", 0)
	p.State = incident.Settling
	p.Journal.Timers = incident.Timers{At: testEpoch, ScalingHeld: true}
	end := testEpoch.Add(10 * time.Minute)
	s := newTestServer()
	s.SetIncidents(&pagingIncidents{
		fakeIncidents: fakeIncidents{items: []incident.Incident{p}},
		scaling: incident.ScalingState{Held: true, Since: testEpoch,
			Expected: end}})

	w := get(t, s, "/api/incidents/a")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got IncidentDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.NotNil(t, got.Why)
	assert.True(t, got.Why.PagingHeldUntil.Equal(end),
		"until %v", got.Why.PagingHeldUntil)

	s.SetIncidents(&pagingIncidents{
		fakeIncidents: fakeIncidents{items: []incident.Incident{p}}})
	w = get(t, s, "/api/incidents/a")
	assert.NotContains(t, w.Body.String(), "pagingHeldUntil")
}
