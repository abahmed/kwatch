package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A release whose template stops naming a Service is recorded as a
// change of the workload once the flap window passes.
func TestEndedLinksRecordsADroppedTemplateCall(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	svc := inventory.CoreID(kube.KindService, "shop", "payments")
	at := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	var links endedLinks
	observe := func(when time.Time, attrs map[string]inventory.Value) {
		_, err := links.apply(model, inventory.Observation{
			Kind: inventory.Observed, Source: kube.ObservationSource,
			At: when, Entity: api, Attributes: attrs})
		require.NoError(t, err)
	}
	observe(at, map[string]inventory.Value{
		kube.AttrTemplateCalls: inventory.Text(svc.String())})
	observe(at.Add(time.Minute), nil)

	assert.Empty(t, links.due(model, at.Add(time.Minute+30*time.Second)))
	got := links.due(model, at.Add(2*time.Minute))
	require.Len(t, got, 1)
	rel, target, ok := got[0].Change.RemovedLink()
	require.True(t, ok)
	assert.Equal(t, inventory.Calls, rel)
	assert.Equal(t, svc, target)
	assert.Equal(t, api, got[0].Entity)
}
