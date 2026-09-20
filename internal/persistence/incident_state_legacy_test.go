package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// v0.10.x wrote incident state as a JSON object keyed by id whose values were
// full model.Incident values; the reader always expected a []PersistedIncident.
// The mismatch meant no restore ever succeeded, and the first upgrade produced
// exactly this error in production. This reproduces it and proves recovery.
func TestLegacyIncidentStateIsMigratedOnLoad(t *testing.T) {
	ctx := context.Background()
	sm := newTestManager(fake.NewSimpleClientset(), "kwatch")
	now := time.Now().UTC().Truncate(time.Second)

	legacy := map[string]any{
		"b-second": map[string]any{
			"Key":       "ns:dep-b:OOMKilled:",
			"Reason":    "OOMKilled",
			"Namespace": "ns",
			"Name":      "dep-b",
			"Count":     7,
			"FirstSeen": now,
		},
		"a-first": map[string]any{
			"Key":       "ns:dep-a:CrashLoopBackOff:",
			"Reason":    "CrashLoopBackOff",
			"Namespace": "ns",
			"Name":      "dep-a",
			"Count":     2,
			"FirstSeen": now,
		},
	}
	require.NoError(t, sm.SaveIncidents(ctx, legacy))

	var direct []model.PersistedIncident
	err := sm.GetIncidents(ctx, &direct)
	require.Error(
		t,
		err,
		"the untyped reader must still fail on the legacy shape — that is the "+
			"bug",
	)
	assert.Contains(
		t,
		err.Error(),
		"cannot unmarshal object into Go value of type "+
			"[]model.PersistedIncident",
	)

	got, err := sm.LoadPersistedIncidents(ctx)
	require.NoError(t, err, "the typed loader must recover the legacy shape")
	require.Len(t, got, 2)
	assert.Equal(
		t,
		"dep-a",
		got[0].Name,
		"restores are sorted by key, not map order",
	)
	assert.Equal(t, "dep-b", got[1].Name)
	assert.Equal(t, 7, got[1].Count)

	// Once re-saved in the current shape, no migration is needed.
	require.NoError(t, sm.SavePersistedIncidents(ctx, got))
	again, err := sm.LoadPersistedIncidents(ctx)
	require.NoError(t, err)
	assert.Equal(t, got, again)
}

// baselineObservation is one crash-looping container of dep-1, as the pod
// pipeline would report it.
func baselineObservation(pod string) *model.Observation {
	obs := observe.PodOwnedBy(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: pod, Namespace: "default",
			},
		},
		"app", "CrashLoopBackOff",
		model.ObjectRef{Namespace: "default", Name: "dep-1"},
	)
	obs.ContainerState = &model.ContainerState{RestartCount: 1}
	return obs
}
