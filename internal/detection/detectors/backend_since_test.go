package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// An admission webhook whose backend Deployment was scaled to zero long
// before kwatch started is dated from that scale-down, the API server's
// record, not from kwatch's first sight of it.
func TestWebhookBackendScaledToZeroKeepsItsAge(t *testing.T) {
	m, id := webhook("Ignore", true, []float64{0})
	svc := newID(kube.KindService, "default", "hook-svc")
	put(m, svc, t0, map[string]inventory.Value{
		kube.AttrSelector: inventory.Text("app=hook")})
	scaled := t0.Add(-435 * 24 * time.Hour)
	put(m, newID(kube.KindDeployment, "default", "hook"), t0,
		map[string]inventory.Value{
			kube.AttrReplicas:       inventory.Number(0),
			kube.AttrTemplateLabels: inventory.Text("app=hook"),
			kube.AttrSpecWritten:    inventory.Time(scaled),
		})

	got := evaluate(Webhook{}, m, t0.Add(time.Minute), id, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, reasons.WebhookNoEndpoints, got[0].Reason)
	assert.True(t, scaled.Equal(got[0].Since), got[0].Since.String())
}

// conversionCRD is a custom resource definition whose conversion webhook
// calls Service conv, which was deleted at gone.
func conversionCRD(gone time.Time) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	crd := newID(kindCRD, "", "widgets.acme.example")
	svc := newID(kube.KindService, "acme", "conv")
	put(m, crd, t0, map[string]inventory.Value{
		kube.AttrCustom: inventory.Bool(true)})
	link(m, crd, inventory.Serves, svc)
	put(m, svc, gone.Add(-time.Hour), nil)
	m.Apply(inventory.Observation{Kind: inventory.Gone, Source: "test",
		At: gone, Entity: svc})
	return m, crd
}

// A conversion webhook whose backend is gone breaks nothing until a
// client needs a conversion: it is informational, dated by the deletion
// and grouped with what its backend left behind.
func TestConversionBackendGoneIsALeftover(t *testing.T) {
	gone := t0.Add(-30 * time.Minute)
	m, crd := conversionCRD(gone)

	got := evaluate(Custom{}, m, t0, crd, nil).Findings

	require.Len(t, got, 1)
	f := got[0]
	assert.Equal(t, reasons.WebhookBackendNotFound, f.Reason)
	assert.Equal(t, detection.Info, f.Severity)
	assert.True(t, gone.Equal(f.Since), f.Since.String())
	assert.Equal(t, newID(kube.KindService, "acme", "conv"), f.Leftover)
}
