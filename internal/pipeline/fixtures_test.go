package pipeline

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/storage"
)

var errStore = errors.New("store failure")

// memStore is an IncidentStore that records calls and can fail.
type memStore struct {
	mu           sync.Mutex
	records      []incident.Record
	prints       map[string]string
	saved        int
	savedPrints  int
	loadErr      error
	printsErr    error
	saveErr      error
	savePrintErr error
	startup      *StartupState
	startupErr   error
	startups     []StartupState
	// lastSaved holds the records of the latest SaveIncidents call.
	lastSaved []incident.Record
	// entered, when set, receives a value as each SaveIncidents starts;
	// gate, when set, blocks it until closed.
	entered chan struct{}
	gate    chan struct{}
}

func (m *memStore) LoadIncidents() ([]incident.Record, error) {
	return m.records, m.loadErr
}

func (m *memStore) SaveIncidents(records []incident.Record) error {
	if m.entered != nil {
		m.entered <- struct{}{}
	}
	if m.gate != nil {
		<-m.gate
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saved++
	m.lastSaved = records
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

func (m *memStore) LoadStartup() (StartupState, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.startup == nil {
		return StartupState{}, false, m.startupErr
	}
	return *m.startup, true, m.startupErr
}

func (m *memStore) SaveStartup(state StartupState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startups = append(m.startups, state)
	m.startup = &state
	return nil
}

func (m *memStore) setSaveErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveErr = err
}

func (m *memStore) latest() []incident.Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastSaved
}

func (m *memStore) counts() (int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saved, m.savedPrints
}

type sinkLog struct {
	mu        sync.Mutex
	decisions []incident.Decision
}

func (s *sinkLog) sink(
	_ context.Context, d incident.Decision, m notification.Message,
) {
	if m.Carrier != "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decisions = append(s.decisions, d)
}

func (s *sinkLog) all() []incident.Decision {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]incident.Decision(nil), s.decisions...)
}

func newTestEngine(
	t *testing.T, clock Clock, sink Sink, mutate func(*Dependencies),
) *Engine {
	t.Helper()
	deps := Dependencies{
		Model:     inventory.NewModel(inventory.Options{}),
		Detectors: detection.NewRegistry(nil, builtinDetectors()...),
		Incidents: incident.NewManager(incident.Config{}, nil),
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

func openTempStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.Open(t.TempDir()+"/state.db", storage.Options{
		Now: func() time.Time { return time.Unix(1_800_000_000, 0) },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := s.Claim(); err != nil {
		t.Fatal(err)
	}
	return s
}
