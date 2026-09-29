package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/store"
	"github.com/abahmed/kwatch/internal/notice"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/signal"
)

var errStore = errors.New("store failure")

// memStore is a ProblemStore that records calls and can fail.
type memStore struct {
	mu           sync.Mutex
	records      []problem.Record
	prints       map[string]string
	saved        int
	savedPrints  int
	loadErr      error
	printsErr    error
	saveErr      error
	savePrintErr error
}

func (m *memStore) LoadProblems() ([]problem.Record, error) {
	return m.records, m.loadErr
}

func (m *memStore) SaveProblems([]problem.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saved++
	return m.saveErr
}

func (m *memStore) LoadFingerprints() (map[string]string, error) {
	return m.prints, m.printsErr
}

func (m *memStore) SaveFingerprints(map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.savedPrints++
	return m.savePrintErr
}

func (m *memStore) counts() (int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saved, m.savedPrints
}

type sinkLog struct {
	mu        sync.Mutex
	decisions []problem.Decision
}

func (s *sinkLog) sink(
	_ context.Context, d problem.Decision, _ notice.Message,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decisions = append(s.decisions, d)
}

func (s *sinkLog) all() []problem.Decision {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]problem.Decision(nil), s.decisions...)
}

func newTestEngine(
	t *testing.T, clock Clock, sink Sink, mutate func(*Dependencies),
) *Engine {
	t.Helper()
	deps := Dependencies{
		Model:     knowledge.NewModel(knowledge.Options{}),
		Detectors: signal.NewRegistry(nil, detectors()...),
		Problems:  problem.NewManager(problem.Config{}, nil),
		Sink:      sink,
		Clock:     clock,
	}
	if mutate != nil {
		mutate(&deps)
	}
	engine, err := NewEngine(deps)
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func openTempStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir()+"/state.db", store.Options{
		Now: func() time.Time { return time.Unix(1_800_000_000, 0) },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Claim(1); err != nil {
		t.Fatal(err)
	}
	return s
}
