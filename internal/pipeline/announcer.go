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
	})
	return a
}

// announce hands the decisions of one tick to people, in order. It
// reports whether it sent a startup summary, a digest, a roll-up, a
// listing's resolve or a held announcement: messages of their own that
// the incidents' decisions did not make, and that change saved state.
func (a *announcer) announce(
	ctx context.Context, now time.Time, decisions []incident.Decision,
) bool {
	quiet := a.incidents.TakeQuietResolves()
	a.dropHeld(quiet)
	a.collect.NoteQuietResolves(quiet)
	a.collect.NoteDecisions(decisions)
	decisions = a.inScope(decisions)
	decisions, summarised := a.collect.CollectStartup(ctx, now, decisions)
	decisions, digested := a.collect.CollectDigest(ctx, now, decisions)
	decisions, grouped := a.collect.CollectOutages(ctx, now, decisions)
	decisions, rolled := a.collect.CollectRollup(ctx, now, decisions)
	expired := a.expireHeld(ctx, now)
	a.deliver(ctx, now, decisions)
	closed := a.collect.CloseListings(ctx)
	listed := a.collect.ListRestored(ctx, now)
	return summarised || digested || grouped || rolled || closed ||
		listed || expired
}

// outputs is the channel finished investigations arrive on; nil, which
// never delivers, without an investigator.
func (a *announcer) outputs() <-chan investigationResult {
	if a.pool == nil {
		return nil
	}
	return a.pool.results
}
