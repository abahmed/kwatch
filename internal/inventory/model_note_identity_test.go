package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func observePod(t *testing.T, m *Model, id EntityID, uid string) {
	t.Helper()
	_, err := m.Apply(Observation{
		Kind: Observed, Source: "k", At: testTime, Entity: id, UID: uid,
	})
	require.NoError(t, err)
}

func noteFor(
	t *testing.T, m *Model, id EntityID, note Note,
) {
	t.Helper()
	_, err := m.Apply(Observation{
		Kind: Noted, Source: "k", At: testTime, Entity: id, Note: note,
	})
	require.NoError(t, err)
}

func TestNotesAboutAnOlderObjectAreDropped(t *testing.T) {
	m := NewModel(Options{})
	pod := CoreID("pod", "db", "pod-0")
	observePod(t, m, pod, "uid-new")

	noteFor(t, m, pod, Note{Reason: "FailedMount", UID: "uid-old",
		Warning: true})
	assert.Empty(t, m.Notes(pod, time.Time{}), "late event of the old pod")

	noteFor(t, m, pod, Note{Reason: "FailedMount", UID: "uid-new",
		Warning: true})
	assert.Len(t, m.Notes(pod, time.Time{}), 1)
}

func TestRecreatedEntityDoesNotInheritNotes(t *testing.T) {
	m := NewModel(Options{})
	pod := CoreID("pod", "db", "pod-0")
	observePod(t, m, pod, "uid-old")
	noteFor(t, m, pod, Note{Reason: "FailedMount", UID: "uid-old",
		Warning: true})
	m.MarkHealth(pod, true, "Pending", testTime)

	_, err := m.Apply(Observation{Kind: Gone, Source: "k", At: testTime,
		Entity: pod})
	require.NoError(t, err)
	observePod(t, m, pod, "uid-new")

	assert.Empty(t, m.Notes(pod, time.Time{}))
	assert.Empty(t, m.HealthMarks(pod, time.Time{}))
}

func TestNoteForANewObjectSurvivesItsFirstObservation(t *testing.T) {
	m := NewModel(Options{})
	pod := CoreID("pod", "db", "pod-0")
	observePod(t, m, pod, "uid-old")
	_, err := m.Apply(Observation{Kind: Gone, Source: "k", At: testTime,
		Entity: pod})
	require.NoError(t, err)
	// The event of the new pod arrives before the pod itself.
	noteFor(t, m, pod, Note{Reason: "FailedMount", UID: "uid-new",
		Warning: true})
	observePod(t, m, pod, "uid-new")

	assert.Len(t, m.Notes(pod, time.Time{}), 1)
}

func TestNotesOfOneReasonSumTheirEventObjects(t *testing.T) {
	m := NewModel(Options{})
	pod := CoreID("pod", "db", "pod-0")
	observePod(t, m, pod, "u")
	for _, n := range []Note{
		{Reason: "BackOff", UID: "u", Origin: "e1", Count: 1, Message: "a"},
		{Reason: "BackOff", UID: "u", Origin: "e2", Count: 2, Message: "b"},
		// The same Event object updated: replaces its own count.
		{Reason: "BackOff", UID: "u", Origin: "e1", Count: 4, Message: "c"},
	} {
		noteFor(t, m, pod, n)
	}
	notes := m.Notes(pod, time.Time{})
	require.Len(t, notes, 1)
	assert.Equal(t, 6, notes[0].Count)
	assert.Equal(t, "c", notes[0].Message)
}

func TestNotesWithoutAnOriginKeepTheLatest(t *testing.T) {
	m := NewModel(Options{})
	pod := CoreID("pod", "db", "pod-0")
	observePod(t, m, pod, "u")
	noteFor(t, m, pod, Note{Reason: "BackOff", Count: 1})
	noteFor(t, m, pod, Note{Reason: "BackOff", Count: 3})
	notes := m.Notes(pod, time.Time{})
	require.Len(t, notes, 1)
	assert.Equal(t, 3, notes[0].Count)
}

func TestContainerNotesFollowTheUIDOfTheirPod(t *testing.T) {
	m := NewModel(Options{})
	pod := CoreID("pod", "db", "pod-0")
	container := CoreID("container", "db", "pod-0/app")
	observePod(t, m, pod, "uid-new")
	observePod(t, m, container, "")
	_, err := m.Apply(Observation{Kind: Related, Source: "k",
		Entity: container, Relation: PartOf, Targets: []EntityID{pod}})
	require.NoError(t, err)

	noteFor(t, m, container, Note{Reason: "Unhealthy", UID: "uid-old"})
	assert.Empty(t, m.Notes(container, time.Time{}))
	noteFor(t, m, container, Note{Reason: "Unhealthy", UID: "uid-new"})
	assert.Len(t, m.Notes(container, time.Time{}), 1)
}
