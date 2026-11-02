package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The kind names a decision arrived with may be the incident's own map:
// adding the model's spellings never writes into it.
func TestAnnouncerWithKindNamesLeavesTheSharedMapAlone(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	root := inventory.NewEntityID("argoproj.io", "rollout", "shop", "api")
	_, err := model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: kube.ObservationSource,
		At: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Entity: root,
		Attributes: map[string]inventory.Value{
			kube.AttrKindName: inventory.Text("Rollout")},
	})
	require.NoError(t, err)
	shared := map[inventory.Kind]string{"widget": "Widget"}
	d := incident.Decision{Incident: incident.Incident{Root: root}}
	d.Facts.KindNames = shared

	got := (&announcer{history: model}).withKindNames(d)

	assert.Equal(t, map[inventory.Kind]string{"widget": "Widget"}, shared)
	assert.Equal(t, map[inventory.Kind]string{
		"widget": "Widget", "rollout": "Rollout"}, got.Facts.KindNames)
}
