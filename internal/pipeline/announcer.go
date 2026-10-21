package pipeline

import (
	"context"
	"os"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification/compose"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
	"github.com/abahmed/kwatch/internal/pipeline/investigate"
)

// announcer decides when and how each incident decision reaches people.
// It drops decisions nobody wants to hear about, collects the incidents
// that already existed at a cold start into one startup summary, rolls
// digest-tier incidents into one message per half hour, sends the
// announcements of one moment as one roll-up (one per namespace when
// many of its workloads fail without a shared cause), holds an
// announcement for a few seconds while its investigation runs, and then
// writes the message and hands it to the sink.
//
// The announcer runs on the decision loop and does no I/O itself. Its
// investigation pool reads logs and the API on separate goroutines, and
// the startup marker is saved through the engine's persistence.
type announcer struct {
	incidents *incident.Manager
	// history is the model's change record: what changed next to an
	// incident that has no cause (see withChanges).
	history  inventory.HistoryReader
	messages compose.Writer
	sink     Sink
	// scope reports whether people want to hear about an incident; nil
	// keeps every decision.
	scope        func(incident.Incident) bool
	investigator investigate.Investigator
	// pool investigates incidents off the loop; nil without an
	// investigator.
	pool *investigationPool
	// held are the announcements waiting for their investigation, in
	// decision order.
	held []heldAnnouncement
	// evidence is what investigation found, by incident ID.
	evidence map[string]*evidenceState
	// collect holds the startup summary, digest, outage and roll-up
	// steps (see the announce package).
	collect *announce.Collector
	// joint holds, while one tick is delivered, the resolves that the
	// resolve of the incident they are keyed by also tells people.
	joint   map[string][]incident.Decision
	storage *persistence
	stats   *workerStats
}

func newAnnouncer(
	deps Dependencies, storage *persistence, stats *workerStats,
) *announcer {
	a := &announcer{
		incidents:    deps.Incidents,
		messages:     deps.Writer,
		sink:         deps.Sink,
		scope:        deps.InScope,
		investigator: deps.Investigator,
		evidence:     map[string]*evidenceState{},
		storage:      storage,
		stats:        stats,
	}
	if deps.Model != nil {
		a.history = deps.Model
	}
	if deps.Investigator != nil {
		a.pool = newInvestigationPool(stats)
	}
	a.collect = announce.New(announce.Env{
		Incidents: deps.Incidents, Messages: deps.Writer,
		History: a.history, Scope: deps.InScope, Sink: deps.Sink,
		Write: a.write, WithChanges: a.withChanges,
		SaveStartup:  storage.saveStartup,
		OwnNamespace: os.Getenv("POD_NAMESPACE"),
		Withheld:     deps.Withheld.Counts,
	})
	return a
}

// announce hands the decisions of one tick to people, in order. It
// reports whether it sent a startup summary, a digest, a roll-up, a
// listing's resolve or a held announcement: messages of their own that
// the incidents' decisions did not make, and that change saved state.
//
// Each collecting step is contained on its own. One that panics takes
// nothing: the decisions go on to the next step and are delivered, so a
// bad digest or listing cannot swallow the pages and updates of the tick.
func (a *announcer) announce(
	ctx context.Context, now time.Time, decisions []incident.Decision,
) bool {
	quiet := a.incidents.TakeQuietResolves()
	a.dropHeld(quiet)
	a.collect.NoteQuietResolves(quiet)
	a.collect.NoteDecisions(decisions)
	decisions = a.inScope(decisions)
	c := a.collect
	decisions, summarised := a.collected(ctx, now, "startup", decisions,
		c.CollectStartup)
	decisions, digested := a.collected(ctx, now, "digest", decisions,
		c.CollectDigest)
	decisions, grouped := a.collected(ctx, now, "outage", decisions,
		c.CollectOutages)
	decisions, rolled := a.collected(ctx, now, "rollup", decisions,
		c.CollectRollup)
	var expired, closed, listed bool
	contain("announce", "held", func() { expired = a.expireHeld(ctx, now) })
	a.deliver(ctx, now, a.splitRecoveries(decisions))
	contain("announce", "joint", func() { a.flushJoint(ctx, now) })
	contain("announce", "close", func() { closed = c.CloseListings(ctx) })
	contain("announce", "restored",
		func() { listed = c.ListRestored(ctx, now) })
	return summarised || digested || grouped || rolled || closed ||
		listed || expired
}

// collectingStep is a step of the announce collector: it takes the
// decisions of the tick and returns the ones that are left for delivery.
type collectingStep func(
	context.Context, time.Time, []incident.Decision,
) ([]incident.Decision, bool)

// collected runs one collecting step, contained. A step that panics is
// counted and logged, and reports that it did nothing: the decisions it
// was given are returned unchanged.
func (a *announcer) collected(
	ctx context.Context, now time.Time, stage string,
	decisions []incident.Decision, step collectingStep,
) (rest []incident.Decision, did bool) {
	rest = decisions
	if !contain("announce", stage, func() {
		rest, did = step(ctx, now, decisions)
	}) {
		return decisions, false
	}
	return rest, did
}

// outputs is the channel finished investigations arrive on; nil, which
// never delivers, without an investigator.
func (a *announcer) outputs() <-chan investigationResult {
	if a.pool == nil {
		return nil
	}
	return a.pool.results
}
