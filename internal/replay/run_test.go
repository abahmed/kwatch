package replay_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/pipeline"
	"github.com/abahmed/kwatch/internal/replay"
)

func readBytes(t *testing.T, data []byte) replay.Log {
	t.Helper()
	log, err := replay.Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return log
}

// The recovery hold elapses on the simulated clock: the incident resolves
// without any wall-clock wait.
func TestRunTicksThroughTheRecoveryHold(t *testing.T) {
	log := readBytes(t, recordRollout(t, 5*time.Minute))
	started := time.Now()
	result, err := replay.Run(context.Background(), log, newDependencies(),
		replay.Options{Tail: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 30*time.Second {
		t.Fatalf("replay of an hour took %s", elapsed)
	}
	got := actions(result)
	if !strings.HasPrefix(got, "announce") ||
		!strings.HasSuffix(got, "resolve") {
		t.Fatalf("decisions = %q, want announce ... resolve", got)
	}
	last := result.Decisions[len(result.Decisions)-1]
	if last.Incident.State != incident.Resolved {
		t.Fatalf("last state %v", last.Incident.State)
	}
	wantEnd := log.Entries[len(log.Entries)-1].At.Add(time.Hour)
	if !result.End.Equal(wantEnd) {
		t.Fatalf("end %s, want %s", result.End, wantEnd)
	}
}

func TestRunForwardsToTheCallerSink(t *testing.T) {
	deps := newDependencies()
	forwarded := 0
	deps.Sink = func(context.Context, incident.Decision,
		notification.Message) {
		forwarded++
	}
	result, err := replay.Run(context.Background(),
		readBytes(t, recordBadRollout(t)), deps, replay.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if forwarded == 0 || forwarded != len(result.Decisions) {
		t.Fatalf("forwarded %d of %d decisions", forwarded,
			len(result.Decisions))
	}
	if len(result.Incidents) == 0 {
		t.Fatal("expected tracked incidents in the result")
	}
}

func TestRunEmptyLogEndsAfterTheTail(t *testing.T) {
	log := replay.Log{Start: rolloutStart}
	result, err := replay.Run(context.Background(), log, newDependencies(),
		replay.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Decisions) != 0 ||
		!result.End.Equal(rolloutStart.Add(replay.DefaultTail)) {
		t.Fatalf("result %+v", result)
	}
}

func TestRunRejectsInvalidInput(t *testing.T) {
	if _, err := replay.Run(context.Background(), replay.Log{},
		newDependencies(), replay.Options{}); err == nil {
		t.Fatal("expected an error for a log without start")
	}
	deps := newDependencies()
	deps.Model = nil
	if _, err := replay.Run(context.Background(),
		replay.Log{Start: rolloutStart}, deps, replay.Options{}); err == nil {
		t.Fatal("expected an error for incomplete dependencies")
	}
}

func TestRunStopsAtTheStepBound(t *testing.T) {
	_, err := replay.Run(context.Background(),
		readBytes(t, recordBadRollout(t)), newDependencies(),
		replay.Options{MaxSteps: 3})
	if err == nil || !strings.Contains(err.Error(), "steps") {
		t.Fatalf("err = %v, want the step bound", err)
	}
}

func TestRunStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := replay.Run(ctx, readBytes(t, recordBadRollout(t)),
		newDependencies(), replay.Options{})
	if err == nil {
		t.Fatal("expected the cancellation error")
	}
}

// An engine that fails to start ends the replay with its error.
func TestRunReportsAnEngineThatStopsEarly(t *testing.T) {
	deps := newDependencies()
	deps.Store = failingStore{}
	_, err := replay.Run(context.Background(),
		readBytes(t, recordBadRollout(t)), deps, replay.Options{})
	if err == nil {
		t.Fatal("expected the restore error")
	}
}

type failingStore struct{}

func (failingStore) LoadIncidents() ([]incident.Record, error) {
	return nil, errWrite
}

func (failingStore) SaveIncidents([]incident.Record) error { return nil }

func (failingStore) LoadFingerprints() (map[string]string, error) {
	return nil, nil
}

func (failingStore) SaveFingerprints(map[string]any) error { return nil }

func (failingStore) LoadStartup() (pipeline.StartupState, bool, error) {
	return pipeline.StartupState{}, false, nil
}

func (failingStore) SaveStartup(pipeline.StartupState) error { return nil }

// memoryStore keeps the last saved incidents.
type memoryStore struct {
	mu    sync.Mutex
	saves int
	last  []incident.Record
}

func (s *memoryStore) LoadIncidents() ([]incident.Record, error) {
	return nil, nil
}

func (s *memoryStore) SaveIncidents(records []incident.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	s.last = records
	return nil
}

func (s *memoryStore) LoadFingerprints() (map[string]string, error) {
	return nil, nil
}

func (s *memoryStore) SaveFingerprints(map[string]any) error { return nil }

func (s *memoryStore) LoadStartup() (pipeline.StartupState, bool, error) {
	return pipeline.StartupState{}, false, nil
}

func (s *memoryStore) SaveStartup(pipeline.StartupState) error { return nil }

// Workers run on simulated time: the caller's wall timer is never used,
// and the final incidents are written before Run returns.
func TestRunGivesWorkersTheSimulatedTimer(t *testing.T) {
	log := readBytes(t, recordRollout(t, 5*time.Minute))
	deps := newDependencies()
	store := &memoryStore{}
	deps.Store = store
	deps.Timer = func(time.Duration) <-chan time.Time {
		t.Error("replay used the wall timer")
		return nil
	}
	result, err := replay.Run(context.Background(), log, deps,
		replay.Options{Tail: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.saves == 0 || len(store.last) != len(result.Incidents) {
		t.Fatalf("saved %d times, last %d records, want %d", store.saves,
			len(store.last), len(result.Incidents))
	}
}
