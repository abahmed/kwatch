package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// freshGated raises a crash on every pod while its kind is fresh and
// holds, asking again in a minute, while it is not.
type freshGated struct{}

func (freshGated) Name() string { return "fresh-gated" }

func (freshGated) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindPod}
}

func (freshGated) Detect(
	ctx detection.Context, _ inventory.Entity,
) []detection.Finding {
	ctx.RecheckAfter(time.Minute)
	if !ctx.Fresh(kube.KindPod) {
		return nil
	}
	return []detection.Finding{{Reason: reasons.CrashLoopBackOff,
		Severity: detection.Warning, Summary: "crashing"}}
}

// Findings the evaluation holds over a kind kwatch cannot verify are
// noted with the incident, which says so once the hold lasts, and no
// longer once the kind can be read again.
func TestHeldFindingsMarkTheIncidentUnverified(t *testing.T) {
	fresh := true
	start := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	h := newHarnessWith(t, start, func(d *Dependencies) {
		d.Detectors = detection.NewRegistry(nil, freshGated{})
		d.Detectors.SetFresh(func(inventory.Kind) bool { return fresh })
	})
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	h.add(kube.PodSchema{}, crashingPod("shop-1", "", "n1", start))
	h.run(h.now.Add(5*time.Minute), 10*time.Second)
	m := h.engine.deps.Incidents
	live := m.Incidents()
	require.Len(t, live, 1)
	id := live[0].ID
	require.Empty(t, m.Unverified(id, h.now))

	fresh = false
	h.run(h.now.Add(incident.UnverifiedAfter+time.Minute), 10*time.Second)
	assert.Contains(t, m.Unverified(id, h.now), "kwatch can't verify")
	require.Len(t, m.Incidents(), 1, "held, still open")

	fresh = true
	h.run(h.now.Add(2*time.Minute), 10*time.Second)
	assert.Empty(t, m.Unverified(id, h.now))
}
