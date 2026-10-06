package inventory

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoteWithAltUIDIsAboutThisObject(t *testing.T) {
	m := NewModel(Options{})
	pod := CoreID("pod", "kube-system", "etcd-n1")
	_, err := m.Apply(Observation{
		Kind: Observed, Source: "k", At: testTime, Entity: pod,
		UID: "mirror-uid", AltUID: "config-hash",
	})
	require.NoError(t, err)

	noteFor(t, m, pod, Note{Reason: "Unhealthy", UID: "config-hash",
		Warning: true})
	noteFor(t, m, pod, Note{Reason: "Other", UID: "stranger",
		Warning: true})

	notes := m.Notes(pod, testTime.Add(-1))
	require.Len(t, notes, 1)
	assert.Equal(t, "Unhealthy", notes[0].Reason)
}

func TestNotedKindsIgnoresNotesAboutOlderObjects(t *testing.T) {
	m := NewModel(Options{})
	pod := CoreID("pod", "db", "pod-0")
	observePod(t, m, pod, "uid-new")
	// A note stored while the entity was unknown, then adopted: simulate
	// by a note that passes the filter and an object swap afterwards.
	noteFor(t, m, pod, Note{Reason: "X", UID: "uid-new", Warning: true})
	assert.True(t, m.NotedKinds(testTime.Add(-1))["pod"])
	observePod(t, m, pod, "uid-newer")
	assert.False(t, m.NotedKinds(testTime.Add(-1))["pod"])
}

func TestFirstObservationPurgesNotesOfAnOlderObject(t *testing.T) {
	m := NewModel(Options{})
	pod := CoreID("pod", "db", "pod-0")
	noteFor(t, m, pod, Note{Reason: "Old", UID: "uid-old", Warning: true})
	noteFor(t, m, pod, Note{Reason: "Blank", Warning: true})
	observePod(t, m, pod, "uid-new")

	notes := m.Notes(pod, testTime.Add(-1))
	require.Len(t, notes, 1)
	assert.Equal(t, "Blank", notes[0].Reason)
}

func TestNoteWithoutOriginResetsCountedParts(t *testing.T) {
	m := NewModel(Options{})
	pod := CoreID("pod", "db", "pod-0")
	noteFor(t, m, pod, Note{Reason: "R", UID: "", Origin: "a", Count: 5})
	noteFor(t, m, pod, Note{Reason: "R", Count: 2})
	noteFor(t, m, pod, Note{Reason: "R", Origin: "b", Count: 1})
	notes := m.Notes(pod, testTime.Add(-1))
	require.Len(t, notes, 1)
	assert.Equal(t, 1, notes[0].Count, "the old origin a is forgotten")
}

func TestObservationJSONKeepsCauseAndAltUID(t *testing.T) {
	in := Observation{
		Kind: Changed, Source: "k", At: testTime,
		Entity: CoreID("pod", "db", "p"), AltUID: "hash",
		Change: Change{Cause: "rollout fix"},
	}
	data, err := json.Marshal(in)
	require.NoError(t, err)
	var out Observation
	require.NoError(t, json.Unmarshal(data, &out))
	assert.Equal(t, "rollout fix", out.Change.Cause)
	assert.Equal(t, "hash", out.AltUID)
}

func TestEarlyStaticPodNoteSurvivesFirstObservation(t *testing.T) {
	m := NewModel(Options{})
	pod := CoreID("pod", "kube-system", "etcd-n1")
	noteFor(t, m, pod, Note{Reason: "Unhealthy", UID: "config-hash",
		Warning: true})
	_, err := m.Apply(Observation{
		Kind: Observed, Source: "k", At: testTime, Entity: pod,
		UID: "mirror-uid", AltUID: "config-hash",
	})
	require.NoError(t, err)

	notes := m.Notes(pod, testTime.Add(-1))
	require.Len(t, notes, 1, "the config hash is this object's own id")
}

func TestNewUIDDropsNotesOfTheOldAltUID(t *testing.T) {
	m := NewModel(Options{})
	pod := CoreID("pod", "kube-system", "etcd-n1")
	_, err := m.Apply(Observation{
		Kind: Observed, Source: "k", At: testTime, Entity: pod,
		UID: "uid-1", AltUID: "hash-1",
	})
	require.NoError(t, err)
	noteFor(t, m, pod, Note{Reason: "Unhealthy", UID: "hash-1",
		Warning: true})
	_, err = m.Apply(Observation{
		Kind: Observed, Source: "k", At: testTime, Entity: pod,
		UID: "uid-2", AltUID: "hash-2",
	})
	require.NoError(t, err)

	assert.Empty(t, m.Notes(pod, testTime.Add(-1)))
}
