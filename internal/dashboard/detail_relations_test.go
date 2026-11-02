package dashboard

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

func TestIncidentDetailCarriesRelations(t *testing.T) {
	payments := inventory.CoreID("deployment", "shop", "payments")
	p := incident.Incident{ID: "b", Root: payments, State: incident.Open,
		Tier: incident.Notify,
		CausedBy: []incident.Related{{ID: "a", Title: "Node n1 is down.",
			Tier: incident.Page, State: incident.Open, Told: true}},
		Radius: incident.Radius{Services: 2, Ingresses: 1, Pods: 3,
			Of: payments, Replicas: 4}}

	d := toDetail(p)

	assert.Equal(t, []RelatedIncident{{ID: "a", Title: "Node n1 is down.",
		Severity: "page", State: "open"}}, d.CausedBy)
	assert.Empty(t, d.Causing)
	assert.Equal(t,
		"Affects 2 services, 1 ingress and 3 of 4 pods of payments",
		d.BlastRadius)

	body, err := json.Marshal(d)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(body, &raw))
	assert.Contains(t, raw, "causedBy")
	assert.Contains(t, raw, "blastRadius")
	assert.NotContains(t, raw, "causing", "an empty list is absent")
}

func TestIncidentDetailBlastRadiusIsTheWidestWhenResolved(t *testing.T) {
	p := incident.Incident{ID: "r", State: incident.Resolved,
		Radius: incident.Radius{Pods: 1}}
	p.Journal.Widest = incident.Radius{Pods: 4}

	assert.Equal(t, "Affects 4 pods", toDetail(p).BlastRadius)
}
