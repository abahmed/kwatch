package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// goneBackend is the Service webhook(...) calls in namespace default.
var goneBackend = newID(kube.KindService, "default", "hook-svc")

// An ignoring webhook whose backend is gone blocks nothing: it is a
// leftover of that backend.
func TestWebhookLeftoverNamesItsAbsentBackend(t *testing.T) {
	m, id := webhook("Ignore", false, nil)

	got := evaluate(Webhook{}, m, t0, id, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, detection.Info, got[0].Severity)
	assert.Equal(t, goneBackend, got[0].Leftover)
}

// A webhook that may block requests keeps its own incident and tier.
func TestWebhookThatMayBlockIsNoLeftover(t *testing.T) {
	m, id := webhookWithRules("Fail",
		"default/hook-svc||pods|*|1||")

	got := evaluate(Webhook{}, m, t0, id, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.True(t, got[0].Leftover.IsZero())
}

// An expired certificate nothing uses, in the namespace of a gone
// webhook backend, is a leftover of that backend; one elsewhere is not.
func TestUnusedCertificateBesideGoneBackendIsALeftover(t *testing.T) {
	m, _ := webhook("Ignore", false, nil)
	here := newID(kube.KindSecret, "default", "hook-tls")
	away := newID(kube.KindSecret, "shop", "web-tls")
	for _, id := range []inventory.EntityID{here, away} {
		put(m, id, t0, map[string]inventory.Value{
			kube.AttrCertExpiry: inventory.Time(t0.Add(-time.Hour)),
		})
	}
	ctx := testDetectorContext(m, t0)

	got := Certificate{}.Detect(ctx, entityOf(m, here))
	require.Len(t, got, 1)
	assert.Equal(t, goneBackend, got[0].Leftover)

	got = Certificate{}.Detect(ctx, entityOf(m, away))
	require.Len(t, got, 1)
	assert.True(t, got[0].Leftover.IsZero())
}

// A stuck deletion is a leftover of the gone backend only once it is
// housekeeping: a fresh one is news of its own.
func TestStuckDeletionBesideGoneBackendIsALeftoverWhenOld(t *testing.T) {
	m, _ := webhook("Ignore", false, nil)
	id := newID(kube.KindSecret, "default", "old-token")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrDeleting:      inventory.Bool(true),
		kube.AttrDeletingSince: inventory.Time(t0),
		kube.AttrFinalizers:    inventory.Text("example.com/cleanup"),
	})

	fresh := evaluate(Generic{}, m, t0.Add(2*time.Hour), id, nil).Findings
	old := evaluate(Generic{}, m, t0.Add(2*DefaultLeftoverAge), id,
		nil).Findings

	require.Len(t, fresh, 1)
	assert.True(t, fresh[0].Leftover.IsZero())
	require.Len(t, old, 1)
	assert.Equal(t, goneBackend, old[0].Leftover)
}

// A backend with ready endpoints is not absent, and of two absent
// ones the first by name is the one every leftover names.
func TestLeftoverOfPicksTheFirstAbsentBackend(t *testing.T) {
	m, _ := webhook("Ignore", true, []float64{1})
	ctx := testDetectorContext(m, t0)
	assert.True(t, leftoverOf(ctx, "default").IsZero(),
		"a backend with a ready endpoint is not absent")

	other := newID(kube.KindMutatingWebhook, "", "mutate")
	put(m, other, t0, nil)
	gone := newID(kube.KindService, "default", "a-gone")
	m.Apply(inventory.Observation{
		Kind: inventory.Related, Source: "test", At: t0,
		Entity: other, Relation: inventory.Serves,
		Targets: []inventory.EntityID{
			newID(kube.KindService, "default", "z-gone"), gone},
	})

	assert.Equal(t, gone, leftoverOf(ctx, "default"))
	assert.True(t, leftoverOf(ctx, "shop").IsZero())
}
