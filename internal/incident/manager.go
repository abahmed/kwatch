package incident

import (
	"strconv"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// Explainer solves the areas of failures a change can affect and
// returns the areas it solved again. *explain.Solver is the production
// explainer; it caches areas, so it must see every Apply.
type Explainer interface {
	Solve(s explain.Snapshot, dirty []inventory.EntityID) []explain.Area
}

// Manager owns every incident. It is safe for concurrent use.
type Manager struct {
	mu        sync.Mutex
	cfg       Config
	override  overrides
	explainer Explainer
	incidents map[string]*Incident
	byMember  map[detection.Key]string
	// byRoot maps a root to the ID of its latest incident. The ID is not
	// derived from the root: a revised cause changes the root and keeps
	// the ID.
	byRoot map[string]string
	// seq numbers new incident IDs; nonce tells IDs of one store
	// apart from those of an earlier, reset store (see newID).
	seq   int
	nonce string
	// restoreGrace delays recovery of restored incidents until detectors
	// had time to re-raise their findings.
	restoreGrace time.Time
	// changed collects the incidents an Apply touched, refreshed once
	// at its end.
	changed map[string]bool
	// model is the inventory of the latest Apply. Resolves read its change
	// history to judge how an incident was fixed.
	model inventory.HistoryReader
	// opened lists the IDs of incidents opened since TakeOpened was last
	// called.
	opened []string
	// resolved orders the resolved incidents for expiry and bounds.
	resolved resolvedIndex
	// refingerprint lists restored announced incidents whose stored
	// fingerprint is still to be replaced by the current one (see
	// adoptFingerprints).
	refingerprint []string
}

// NewManager builds a manager that explains failures with explainer.
// A nil explainer takes a fresh explain.Solver.
func NewManager(cfg Config, explainer Explainer) *Manager {
	if explainer == nil {
		explainer = explain.NewSolver()
	}
	nonce := cfg.IDNonce
	if nonce == "" {
		nonce = randomNonce()
	}
	return &Manager{
		cfg: cfg.withDefaults(), explainer: explainer, nonce: nonce,
		override: newOverrides(
			cfg.SeverityByReason, cfg.SeverityByOwnerKind),
		incidents: make(map[string]*Incident),
		byMember:  make(map[detection.Key]string),
		byRoot:    make(map[string]string),
		changed:   make(map[string]bool),
		resolved:  newResolvedIndex(),
	}
}

// Apply folds one loop iteration into incidents. Cleared findings are
// detached. Then every area the explainer solves again is compared with
// the incidents its failures belong to: each failure's findings move to
// the incident of the cause that now covers them. A raised or changed
// finding outside every area, such as an informational one, belongs to
// its own workload.
//
// s holds the active findings after the transitions. dirty lists the
// entities whose relations, notes or changes moved; the entities of
// the transitions are added to it.
func (m *Manager) Apply(
	s explain.Snapshot, dirty []inventory.EntityID,
	transitions []detection.Transition,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.Verifiable == nil {
		s.Verifiable = m.cfg.Verifiable
	}
	if history, ok := s.Model.(inventory.HistoryReader); ok {
		m.model = history
	}
	touched := append([]inventory.EntityID(nil), dirty...)
	for _, t := range transitions {
		touched = append(touched, t.Finding.Entity)
		if t.Kind == detection.Cleared {
			m.detach(s.Now, t.Finding)
		}
	}
	placed := map[detection.Key]bool{}
	for _, area := range m.explainer.Solve(s, touched) {
		m.placeArea(s, area, placed)
	}
	for _, t := range transitions {
		if t.Kind != detection.Cleared && !placed[t.Finding.Key()] {
			own := placement{root: defaultRoot(s.Model, t.Finding.Entity)}
			m.attach(s.Now, t.Finding, own)
		}
	}
	m.adoptAdvisories(s)
	m.refresh(s.Model)
}

// maxTimeline bounds the events an incident keeps. The timeline is
// persisted with the incident and shown to readers, and the oldest events
// of a long-flapping incident say nothing the newest do not.
const maxTimeline = 50

func (inc *Incident) note(at time.Time, text string) {
	inc.Timeline = append(inc.Timeline, Event{At: at, Text: text})
	inc.sent.noted++
	if over := len(inc.Timeline) - maxTimeline; over > 0 {
		inc.Timeline = append(inc.Timeline[:0:0], inc.Timeline[over:]...)
	}
}

func describe(id inventory.EntityID) string {
	if id.Namespace == "" {
		return string(id.Kind) + " " + id.Name
	}
	return string(id.Kind) + " " + id.Namespace + "/" + id.Name
}

// maxOccurrences bounds the recurrence history kept per incident.
const maxOccurrences = 20

// appendOccurrence returns a new slice with at appended, keeping the
// most recent maxOccurrences.
func appendOccurrence(times []time.Time, at time.Time) []time.Time {
	times = append(append([]time.Time(nil), times...), at)
	if over := len(times) - maxOccurrences; over > 0 {
		times = append(times[:0:0], times[over:]...)
	}
	return times
}

func ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return strconv.Itoa(n) + suffix
}

// Incidents returns detached copies of every tracked incident ordered by ID,
// so callers on other goroutines never touch manager state.
func (m *Manager) Incidents() []Incident {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Incident, 0, len(m.incidents))
	for _, id := range m.sortedIDs() {
		out = append(out, m.incidents[id].Snapshot())
	}
	return out
}
