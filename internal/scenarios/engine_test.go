package scenarios

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification/compose"
	"github.com/abahmed/kwatch/internal/pipeline"
	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
	"github.com/abahmed/kwatch/internal/scope"
)

// scenarioCluster is the cluster name scenario messages carry, so every
// verdict covers the cluster tag the composer adds for a named cluster.
const scenarioCluster = "prod-eu-1"

// newDependencies builds fresh engine state composed the way the
// application composes it (internal/app/pipeline.go): the production
// detectors, solver, severity overrides, runbooks, scope and writer with
// a cluster name, plus a store. Every kind counts as synced: the
// simulated cluster lists everything it holds. Kinds in unverifiable are
// reported as not observable, as when kwatch lacks the permission.
//
// One part of production is left out on purpose: the Investigator. Its
// reads run on worker goroutines, and a replay cannot order their
// results against the simulated clock, so verdicts would change from
// run to run. Investigation evidence is covered by the pipeline tests.
func newDependencies(unverifiable ...inventory.Kind) pipeline.Dependencies {
	synced := func(inventory.Kind) bool { return true }
	hidden := map[inventory.Kind]bool{}
	for _, kind := range unverifiable {
		hidden[kind] = true
	}
	policy := config.RuntimeConfigFor(config.DefaultConfig()).Policy()
	model := inventory.NewModel(inventory.Options{})
	return pipeline.Dependencies{
		Model:     model,
		Detectors: detection.NewRegistry(synced, appDetectors()...),
		Incidents: incident.NewManager(incident.Config{
			SeverityByReason:    policy.SeverityByReason(),
			SeverityByOwnerKind: policy.SeverityByOwnerKind(),
			Verifiable: func(kind inventory.Kind) bool {
				return !hidden[kind]
			},
			// A fixed nonce keeps incident IDs, and the order of
			// incidents announced at the same moment, reproducible.
			IDNonce: "5c3e",
		}, explain.NewSolver()),
		Synced:  synced,
		Writer:  compose.NewWriter(scenarioCluster, policy.Runbooks()),
		Store:   &memoryStore{},
		InScope: pipeline.IncidentScope(model, defaultScope()),
	}
}

// appDetectors is the production detector set.
func appDetectors() []detection.Detector {
	return detectors.Default()
}

// defaultScope is the finding scope of the default configuration. It
// has no silences or maintenance windows, so its clock is never read.
func defaultScope() pipeline.FindingScope {
	runtime := config.RuntimeConfigFor(config.DefaultConfig())
	s, err := scope.New(runtime.Scope(), runtime.Scope().Silences(),
		false, func() time.Time { return time.Time{} })
	if err != nil {
		panic("scenarios: default scope: " + err.Error())
	}
	return s
}

// memoryStore is the engine's incident store, in memory. Replays start
// empty and never restart, so it only has to accept writes.
type memoryStore struct {
	mu    sync.Mutex
	saves int
}

func (m *memoryStore) LoadIncidents() ([]incident.Record, error) {
	return nil, nil
}

func (m *memoryStore) SaveIncidents([]incident.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saves++
	return nil
}

func (m *memoryStore) LoadFingerprints() (map[string]string, error) {
	return nil, nil
}

func (m *memoryStore) SaveFingerprints(map[string]any) error { return nil }

func (m *memoryStore) LoadStartup() (pipeline.StartupState, bool, error) {
	return pipeline.StartupState{}, false, nil
}

func (m *memoryStore) SaveStartup(pipeline.StartupState) error { return nil }

// replayLog runs a log through a fresh engine.
func replayLog(
	t testing.TB, log replay.Log, opts replay.Options,
) replay.Result {
	t.Helper()
	result, err := replay.Run(context.Background(), log, newDependencies(),
		opts)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// reported is one incident people heard about, in its final state.
type reported struct {
	ID    string
	Root  string
	Cause *rootcause.CauseRecord
	Tier  incident.Tier
	// First is when people first heard about it.
	First time.Time
}

// confidence is the stated confidence of the cause; zero without one.
func (r reported) confidence() float64 {
	if r.Cause == nil {
		return 0
	}
	return r.Cause.Score
}

// reportedIncidents returns the incidents a replay told people about,
// first announced first, each in its final state. An incident superseded
// by a revised cause is reported as the incident that took it over.
// Incidents announced only inside a startup summary count too.
func reportedIncidents(result replay.Result) []reported {
	final := map[string]incident.Incident{}
	for _, p := range result.Incidents {
		final[p.ID] = p
	}
	first := map[string]time.Time{}
	note := func(id string, at time.Time) {
		if _, seen := first[id]; !seen && id != "" {
			first[id] = at
		}
	}
	for i, d := range result.Decisions {
		note(d.Incident.ID, result.Times[i])
		final[d.Incident.ID] = latest(final[d.Incident.ID], d.Incident)
	}
	for _, p := range result.Incidents {
		if !p.Announced.IsZero() {
			note(p.ID, p.Announced)
		}
	}
	var out []reported
	seen := map[string]bool{}
	for id, at := range first {
		p := successor(final, final[id])
		if seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		out = append(out, reported{
			ID: p.ID, Root: p.Root.String(), Cause: p.Cause, Tier: p.Tier,
			First: at,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].First.Equal(out[j].First) {
			return out[i].First.Before(out[j].First)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// latest keeps the tracked final state unless only the decision knows the
// incident.
func latest(tracked, decided incident.Incident) incident.Incident {
	if tracked.ID == "" {
		return decided
	}
	return tracked
}

func successor(
	final map[string]incident.Incident, p incident.Incident,
) incident.Incident {
	for range 8 {
		next, ok := final[p.SupersededBy]
		if p.SupersededBy == "" || !ok {
			return p
		}
		p = next
	}
	return p
}

// auditEntries turns a replay's messages into audit entries, as the
// application logs them, with incident keys prefixed so the entries of
// several replays can be scored together.
func auditEntries(prefix string, result replay.Result) []audit.Entry {
	entries := make([]audit.Entry, 0, len(result.Decisions))
	for i, d := range result.Decisions {
		entry := pipeline.AuditEntry(d, result.Messages[i], result.Times[i])
		if entry.Incident == "" {
			// Summaries and digests are conversations of their own, one
			// per message key, so several digests never count as one
			// incident with many messages.
			entry.Incident = result.Messages[i].Key
			if entry.Incident == "" {
				entry.Incident = "startup-summary"
			}
			entry.Action = audit.ActionCreate
			if d.Reason == "startup summary resolved" ||
				d.Reason == "roll-up resolved" {
				entry.Action = audit.ActionResolved
			}
		}
		entry.Incident = prefix + "/" + entry.Incident
		if entry.Previous != "" {
			entry.Previous = prefix + "/" + entry.Previous
		}
		entries = append(entries, entry)
	}
	return entries
}
