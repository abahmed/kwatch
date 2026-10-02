package pipeline

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// failingHistory is a HistoryStore whose writes fail until healed.
type failingHistory struct {
	mu        sync.Mutex
	fail      bool
	timeline  []TimelineEntry
	baselines map[string]inventory.Baseline
}

func (f *failingHistory) AppendTimeline(entries []TimelineEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("store failure")
	}
	f.timeline = append(f.timeline, entries...)
	return nil
}

func (f *failingHistory) LoadTimeline(
	string, time.Time, time.Time,
) ([]TimelineEntry, error) {
	return nil, nil
}

func (f *failingHistory) LoadBaselines() (
	map[string]inventory.Baseline, error,
) {
	return nil, errors.New("unreadable")
}

func (f *failingHistory) SaveBaselines(
	values map[string]inventory.Baseline,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("store failure")
	}
	f.baselines = values
	return nil
}

func TestHistoryWriterFillsChangeSetsAndSavesBaselines(t *testing.T) {
	store := NewIncidentStore(openTempStore(t)).(HistoryStore)
	model := inventory.NewModel(inventory.Options{})
	deployment := inventory.CoreID("deployment", "shop", "api")
	change := inventory.Change{Entity: deployment, At: timelineAt(0),
		Fields: []inventory.FieldChange{{Path: "spec.template"}}}
	_, err := model.Apply(inventory.Observation{Kind: inventory.Changed,
		Source: "test", At: change.At, Entity: deployment, Change: change})
	require.NoError(t, err)
	model.Baselines().Add(deployment, inventory.MetricReadySeconds, 12,
		timelineAt(0))

	w := newHistoryWriter(store, model, wallTimer, &workerStats{})
	w.add(changeEntry(change))
	w.flush()

	got, err := store.LoadTimeline(deployment.String(), time.Time{},
		time.Time{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	sets := model.ChangeSets(deployment, time.Time{})
	require.Len(t, sets, 1)
	assert.Equal(t, sets[0].ID, got[0].ChangeSet)
	saved, err := store.LoadBaselines()
	require.NoError(t, err)
	assert.Contains(t, saved, deployment.String())
}

func TestHistoryWriterKeepsFailedBatch(t *testing.T) {
	store := &failingHistory{fail: true}
	model := inventory.NewModel(inventory.Options{})
	workload := inventory.CoreID("deployment", "shop", "api")
	model.Baselines().Add(workload, inventory.MetricJobSeconds, 3,
		timelineAt(0))
	stats := &workerStats{}
	w := newHistoryWriter(store, model, wallTimer, stats)
	w.add(TimelineEntry{Entity: "pod/shop/a", Kind: TimelineEvent})

	w.flush()
	assert.Equal(t, int64(1), stats.writeFailures.Load())
	assert.Len(t, w.pending, 1)

	store.fail = false
	w.flush()
	assert.Len(t, store.timeline, 1)
	assert.Contains(t, store.baselines, workload.String())
	assert.Empty(t, w.pending)
}

func TestHistoryWriterBoundsPendingEntries(t *testing.T) {
	w := newHistoryWriter(&failingHistory{},
		inventory.NewModel(inventory.Options{}), wallTimer, &workerStats{})
	for i := 0; i < maxPendingTimeline+5; i++ {
		w.add(TimelineEntry{Entity: "pod/shop/a", Count: i})
	}
	require.Len(t, w.pending, maxPendingTimeline)
	assert.Equal(t, 5, w.pending[0].Count)
	assert.Equal(t, 5, w.dropped)
}

func TestHistoryWriterWritesOnCloseAndStops(t *testing.T) {
	store := &failingHistory{}
	timer := newBatchTimer()
	w := newHistoryWriter(store, inventory.NewModel(inventory.Options{}),
		timer.after, &workerStats{})
	go w.run()
	waitFor(t, timer.waits)
	w.add(TimelineEntry{Entity: "pod/shop/a"})
	waitFor(t, timer.waits)
	timer.fire <- time.Time{}
	waitFor(t, timer.waits)
	w.add(TimelineEntry{Entity: "pod/shop/b"})

	require.True(t, w.close(nil))
	assert.Len(t, store.timeline, 2)
	assert.True(t, w.close(nil), "close is idempotent")
}

func TestHistoryRecorderRecordsTheLoop(t *testing.T) {
	store := NewIncidentStore(openTempStore(t))
	model := inventory.NewModel(inventory.Options{})
	recorder := newHistoryRecorder(Dependencies{
		Model: model, Store: store, Timer: wallTimer,
	}, &workerStats{})
	recorder.start()
	pod := inventory.CoreID("pod", "shop", "api-1")
	observations := []inventory.Observation{
		{Kind: inventory.Changed, Source: "test", At: timelineAt(0),
			Entity: pod, Change: inventory.Change{Fields: []inventory.FieldChange{
				{Path: "metadata.labels"}}}},
		{Kind: inventory.Noted, Source: "test", At: timelineAt(time.Second),
			Entity: pod, Note: inventory.Note{Source: "events",
				Reason: "BackOff", Warning: true, At: timelineAt(time.Second)}},
		{Kind: inventory.Noted, Source: "test", At: timelineAt(time.Second),
			Entity: pod, Note: inventory.Note{Source: "events",
				Reason: "Pulled"}},
	}
	for _, o := range observations {
		_, err := model.Apply(o)
		require.NoError(t, err)
		recorder.observed(o)
	}
	finding := detection.Finding{Entity: pod, Reason: "CrashLoopBackOff",
		Health: detection.Failing, Since: timelineAt(2 * time.Second)}
	recorder.transitions(timelineAt(3*time.Second), []detection.Transition{
		{Kind: detection.Raised, Finding: finding}},
		activeOf(finding))
	recorder.decided(timelineAt(4*time.Second), []incident.Decision{
		{Action: incident.Announce, Incident: incident.Incident{Root: pod}}})
	recorder.stop(nil)

	got, err := store.(HistoryStore).LoadTimeline(pod.String(),
		time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, []TimelineKind{TimelineChange, TimelineEvent,
		TimelineFinding, TimelineDecision}, kinds(got))
	assert.Equal(t, timelineAt(time.Second), got[1].FirstSeen)
	marks := model.HealthMarks(pod, time.Time{})
	require.Len(t, marks, 1)
	assert.True(t, marks[0].Failing)
}

func TestHistoryRecorderWithoutStoreOnlyMarksHealth(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	recorder := newHistoryRecorder(Dependencies{Model: model},
		&workerStats{})
	recorder.start()
	recorder.stop(nil)
	pod := inventory.CoreID("pod", "shop", "api-1")
	finding := detection.Finding{Entity: pod, Health: detection.Failing}
	recorder.transitions(timelineAt(0), []detection.Transition{
		{Kind: detection.Raised, Finding: finding}}, activeOf(finding))
	recorder.transitions(timelineAt(time.Second), []detection.Transition{
		{Kind: detection.Cleared, Finding: finding}}, activeOf())
	recorder.decided(timelineAt(0), []incident.Decision{{}})
	marks := model.HealthMarks(pod, time.Time{})
	require.Len(t, marks, 2)
	assert.False(t, marks[1].Failing)
}

// activeOf returns an active-findings reader holding found.
func activeOf(
	found ...detection.Finding,
) func(inventory.EntityID) []detection.Finding {
	return func(id inventory.EntityID) []detection.Finding {
		var out []detection.Finding
		for _, f := range found {
			if f.Entity == id {
				out = append(out, f)
			}
		}
		return out
	}
}

func TestHistoryRecorderMarksFromActiveFindings(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	recorder := newHistoryRecorder(Dependencies{Model: model},
		&workerStats{})
	pod := inventory.CoreID("pod", "shop", "api-1")
	crash := detection.Finding{Entity: pod, Reason: "CrashLoopBackOff",
		Mode: "CrashLoop", Health: detection.Failing}
	probe := detection.Finding{Entity: pod, Reason: "Unhealthy",
		Mode: "Probe", Health: detection.Degraded}
	unknown := detection.Finding{Entity: pod, Reason: "Unknown",
		Health: detection.Unknown}

	// The probe finding clears, the crash loop still holds.
	recorder.transitions(timelineAt(0), []detection.Transition{
		{Kind: detection.Cleared, Finding: probe}}, activeOf(crash))
	// Only a finding of unknown health is left.
	recorder.transitions(timelineAt(time.Second), []detection.Transition{
		{Kind: detection.Cleared, Finding: crash}}, activeOf(unknown))

	marks := model.HealthMarks(pod, time.Time{})
	require.Len(t, marks, 1, "unknown health says nothing, so the "+
		"second transition writes no mark")
	assert.True(t, marks[0].Failing, "a failing finding is still active")
	assert.Equal(t, "CrashLoop", marks[0].Mode)

	// Once the unknown finding clears too, the pod is marked healthy.
	recorder.transitions(timelineAt(2*time.Second), []detection.Transition{
		{Kind: detection.Cleared, Finding: unknown}}, activeOf())
	marks = model.HealthMarks(pod, time.Time{})
	require.Len(t, marks, 2)
	assert.False(t, marks[1].Failing)
}

func TestHistoryRecorderStartsWithUnreadableBaselines(t *testing.T) {
	recorder := &historyRecorder{
		model: inventory.NewModel(inventory.Options{}),
		store: &failingHistory{}, timer: wallTimer, stats: &workerStats{},
		now: func() time.Time { return timelineAt(0) },
	}
	recorder.start()
	recorder.stop(nil)
}

// A timeline entry written before its change set merged into an older
// one keeps the younger ID; the model still resolves it to the set.
func TestPersistedChangeSetIDResolvesAfterMerge(t *testing.T) {
	store := NewIncidentStore(openTempStore(t)).(HistoryStore)
	model := inventory.NewModel(inventory.Options{})
	api := inventory.CoreID("deployment", "shop", "api")
	web := inventory.CoreID("deployment", "shop", "web")
	record := func(change inventory.Change) {
		_, err := model.Apply(inventory.Observation{
			Kind: inventory.Changed, Source: "test", At: change.At,
			Entity: change.Entity, Change: change})
		require.NoError(t, err)
	}
	apiChange := inventory.Change{Entity: api, At: timelineAt(0),
		App: "argocd/api", Fields: []inventory.FieldChange{
			{Path: "spec.template"}}}
	webChange := inventory.Change{Entity: web, At: timelineAt(90 * time.Second),
		App: "argocd/web", Fields: []inventory.FieldChange{
			{Path: "spec.template"}}}
	record(apiChange)
	record(webChange)
	w := newHistoryWriter(store, model, wallTimer, &workerStats{})
	w.add(changeEntry(webChange))
	w.flush()

	// One person edits both workloads in between: the two sets merge.
	for _, id := range []inventory.EntityID{api, web} {
		record(inventory.Change{Entity: id, At: timelineAt(time.Minute),
			Actor: "alice", Fields: []inventory.FieldChange{
				{Path: "spec.replicas"}}})
	}

	got, err := store.LoadTimeline(web.String(), time.Time{}, time.Time{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	set, ok := model.ChangeSet(got[0].ChangeSet)
	require.True(t, ok, "the persisted ID %q resolves", got[0].ChangeSet)
	assert.NotEqual(t, set.ID, got[0].ChangeSet,
		"the entry names the absorbed set")
	assert.Contains(t, set.Entities(), web)
	assert.Contains(t, set.Entities(), api)
}
