package pipeline

import (
	"context"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

// announcer decides when and how each incident decision reaches people.
// It drops decisions nobody wants to hear about, collects the incidents
// that already existed at a cold start into one startup summary, rolls
// digest-tier incidents into one message per half hour, sends the
// announcements of one moment as one roll-up, holds an announcement for
// a few seconds while its investigation runs, and then writes the
// message and hands it to the sink.
//
// The announcer runs on the decision loop and does no I/O itself. Its
// investigation pool reads logs and the API on separate goroutines, and
// the startup marker is saved through the engine's persistence.
type announcer struct {
	incidents *incident.Manager
	messages  compose.Writer
	sink      Sink
	// scope reports whether people want to hear about an incident; nil
	// keeps every decision.
	scope        func(incident.Incident) bool
	investigator Investigator
	// pool investigates incidents off the loop; nil without an
	// investigator.
	pool *investigationPool
	// held are the announcements waiting for their investigation, in
	// decision order.
	held []heldAnnouncement
	// evidence is what investigation found, by incident ID.
	evidence map[string]*evidenceState
	startup  startupSummary
	digest   lowDigest
	storage  *persistence
	stats    *workerStats
	// advisories lists the active configuration risks; nil when the
	// engine has none to offer. The digest names each once.
	advisories func() []detection.Finding
	// mentionedRisks are the risks a digest already named.
	mentionedRisks map[detection.Key]bool
}

func newAnnouncer(
	deps Dependencies, storage *persistence, stats *workerStats,
) *announcer {
	a := &announcer{
		incidents:      deps.Incidents,
		messages:       deps.Writer,
		sink:           deps.Sink,
		scope:          deps.InScope,
		investigator:   deps.Investigator,
		evidence:       map[string]*evidenceState{},
		storage:        storage,
		stats:          stats,
		mentionedRisks: map[detection.Key]bool{},
	}
	if deps.Investigator != nil {
		a.pool = newInvestigationPool(stats)
	}
	return a
}

// announce hands the decisions of one tick to people, in order. It
// reports whether it sent a startup summary or the summary's resolve,
// which are decisions of their own that the incidents never made.
func (a *announcer) announce(
	ctx context.Context, now time.Time, decisions []incident.Decision,
) bool {
	a.startup.checkSummary = a.startup.checkSummary || hasResolve(decisions)
	decisions = a.inScope(decisions)
	decisions, summarised := a.collectStartup(ctx, now, decisions)
	decisions, digested := a.collectDigest(ctx, now, decisions)
	decisions, rolled := a.collectRollup(ctx, now, decisions)
	a.expireHeld(ctx, now)
	a.deliver(ctx, now, decisions)
	closed := a.closeSummary(ctx)
	return summarised || digested || rolled || closed
}

// outputs is the channel finished investigations arrive on; nil, which
// never delivers, without an investigator.
func (a *announcer) outputs() <-chan investigationResult {
	if a.pool == nil {
		return nil
	}
	return a.pool.results
}
