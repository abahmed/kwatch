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

// unresolvedModel: reports in shop keeps failing to resolve name; the
// shop namespace holds the Services given.
func unresolvedModel(
	name string, services ...string,
) (*inventory.Model, inventory.EntityID) {
	model := newTestModel()
	put(model, newID(kube.KindNamespace, "", "shop"), t0, nil)
	for _, s := range services {
		put(model, newID(kube.KindService, "shop", s), t0, nil)
	}
	id := newID(kube.KindDeployment, "shop", "reports")
	put(model, id, t0, map[string]inventory.Value{
		kube.AttrLogUnresolvedName:    inventory.Text(name),
		kube.AttrLogUnresolvedSamples: inventory.Number(5),
		kube.AttrLogUnresolvedSince:   inventory.Time(t0),
		kube.AttrLogUnresolvedLine: inventory.Text(
			"Name or service not known (paymnts-api:80)"),
	})
	return model, id
}

func TestLogServiceMissingFlagsAMissingService(t *testing.T) {
	model, id := unresolvedModel("paymnts-api.shop", "payments-api")
	got := evaluate(LogServiceMissing{}, model, t0.Add(time.Minute), id,
		nil)
	require.Len(t, got.Findings, 1)
	f := got.Findings[0]
	assert.Equal(t, reasons.LogServiceMissing, f.Reason)
	assert.Equal(t, detection.Info, f.Severity)
	assert.Equal(t, "keeps trying to reach Service paymnts-api, which "+
		"does not exist", f.Summary)
	assert.Contains(t, f.Evidence, detection.Evidence{
		Label: detection.EvidenceSimilarService, Value: "payments-api"})
}

func TestLogServiceMissingStaysQuiet(t *testing.T) {
	cases := map[string]func() (*inventory.Model, inventory.EntityID){
		"service exists": func() (*inventory.Model, inventory.EntityID) {
			return unresolvedModel("payments-api.shop", "payments-api")
		},
		"namespace unknown": func() (*inventory.Model, inventory.EntityID) {
			return unresolvedModel("example.com")
		},
		"nothing logged": func() (*inventory.Model, inventory.EntityID) {
			return unresolvedModel("")
		},
		"cluster DNS failing": func() (*inventory.Model, inventory.EntityID) {
			model, id := unresolvedModel("paymnts-api.shop")
			put(model, kube.ClusterDNS, t0, map[string]inventory.Value{
				kube.AttrHealthy: inventory.Bool(false)})
			return model, id
		},
	}
	for name, build := range cases {
		model, id := build()
		got := evaluate(LogServiceMissing{}, model, t0.Add(time.Minute),
			id, nil)
		assert.Empty(t, got.Findings, name)
	}
	model, id := unresolvedModel("paymnts-api.shop")
	unsynced := func(kind inventory.Kind) bool {
		return kind != kube.KindService
	}
	got := evaluate(LogServiceMissing{}, model, t0.Add(time.Minute), id,
		unsynced)
	assert.Empty(t, got.Findings, "Services not synced")
}
