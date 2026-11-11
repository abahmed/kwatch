package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// detailContext is what the incident page reads besides the incident:
// the live inventory, the delivery trail and the time. Every part may
// be missing; the page then leaves that part out.
type detailContext struct {
	now   time.Time
	model inventory.HistoryReader
	// changes allows the change history (Pro: analysis.changes).
	changes bool
	trail   []DeliveryAttempt
	// known reports whether an incident ID can still be opened.
	known func(id string) bool
	// similar is how many other incidents have the same error.
	similar int
	// similarRefs names a few of them.
	similarRefs []SimilarRef
	// peers are the incidents kwatch holds, for "at the same time".
	peers []incident.Incident
	// usage is the containers' hourly usage peaks; nil without.
	usage usageSource
	// lookup finds an incident kwatch still keeps, for moved links.
	lookup func(id string) *incident.Incident
	// delivery is what the pagers and channels hold of the incident.
	delivery DeliveryDetail
	// logsForbidden says kwatch may not read pod logs now.
	logsForbidden bool
}

// detailOf is the detail of p with everything the server can add.
func (s *Server) detailOf(p incident.Incident) IncidentDetail {
	c := detailContext{now: s.now.Now(), changes: s.opts.ChangeHistory}
	if m := s.sources.model.Load(); m != nil {
		c.model = *m
	}
	if t := s.sources.trail.Load(); t != nil {
		c.trail = (*t)(p.ID)
	}
	if src := s.sources.delivery.Load(); src != nil {
		c.delivery = (*src)(p.ID)
	}
	if self, ok := s.selfReport(); ok {
		c.logsForbidden = self.PodLogsForbidden
	}
	if r := s.sources.incidents.Load(); r != nil {
		reader := *r
		c.known = func(id string) bool { return reader.Incident(id) != nil }
		c.lookup = reader.Incident
		c.peers = reader.Incidents()
		c.similar = countSimilar(p, c.peers)
		c.similarRefs = similarRefs(p, c.peers)
	}
	if u := s.sources.usage.Load(); u != nil {
		c.usage = *u
	}
	d := buildDetail(p, c)
	if d.Why != nil && d.Why.PagingHeldReason != "" {
		if hold := s.scalingHold(c.now); hold != nil {
			d.Why.PagingHeldUntil = hold.ExpectedEnd
		}
	}
	s.addUnverified(&d, p.ID, c.now)
	d.Release = s.releaseFor(p.ID, p.Opened)
	if c.model != nil {
		d.Map = issueMap(c.model, s.opts.Scope, c.peers, p)
	}
	return d
}

// addContext fills the page's context parts of d. Each list is bounded
// here and counted in d.Truncated when entries were left out.
func (c detailContext) addContext(d *IncidentDetail, p incident.Incident) {
	d.Fix = string(p.Fix)
	d.Similar = c.similar
	d.SimilarIssues = c.similarRefs
	d.SeenBefore = seenBefore(p, c.now, c.known)
	d.Phases = phasesOf(p, c.now)
	addJournal(d, p, c.now)
	addRelations(d, p)
	c.addEnd(d, p)
	var cut int
	d.Deliveries, cut = toDeliveries(c.trail)
	countCut(d, func(t *DetailTruncation) { t.Deliveries += cut })
	d.Affected, cut = affectedOf(p, c.model)
	countCut(d, func(t *DetailTruncation) { t.Affected += cut })
	if c.changes {
		// Stored changes outlive the change history, so they are
		// listed even before the inventory is read.
		d.Changes, d.FixedBy, cut = changesOf(c.model, p, c.now)
		countCut(d, func(t *DetailTruncation) { t.Changes += cut })
	}
	c.addMoment(d, p)
	if c.model == nil {
		return
	}
	addInsight(d, c.model, p)
	d.Containers, cut = containersOf(c.model, p)
	countCut(d, func(t *DetailTruncation) { t.Containers += cut })
	d.UsageHours, cut = usageOf(p, d.Containers, c.usage)
	countCut(d, func(t *DetailTruncation) { t.UsageHours += cut })
}

// addMoment places p in time: when it started, was noticed and told,
// and the other issues that started with it.
func (c detailContext) addMoment(d *IncidentDetail, p incident.Incident) {
	d.Timing = timingOf(p, c.trail)
	var cut int
	d.SameTime, cut = sameTimeOf(p, c.peers, c.model)
	countCut(d, func(t *DetailTruncation) { t.SameTime += cut })
}

// countCut applies add to d's truncation counts, creating them when
// needed; fitDetail drops counts that stay zero.
func countCut(d *IncidentDetail, add func(t *DetailTruncation)) {
	if d.Truncated == nil {
		d.Truncated = &DetailTruncation{}
	}
	add(d.Truncated)
}

// addInsight fills what the live inventory says about p's objects now:
// recent warnings, traffic, nodes, probes and why pods wait.
func addInsight(
	d *IncidentDetail, model inventory.Reader, p incident.Incident,
) {
	var cut int
	d.Warnings, cut = warningsOf(model, p)
	countCut(d, func(t *DetailTruncation) { t.Warnings += cut })
	d.Traffic, cut = trafficOf(model, p)
	countCut(d, func(t *DetailTruncation) { t.Traffic += cut })
	d.Nodes, cut = nodesOf(model, p)
	countCut(d, func(t *DetailTruncation) { t.Nodes += cut })
	d.Probes, cut = probesOf(model, p)
	countCut(d, func(t *DetailTruncation) { t.Probes += cut })
	d.Waiting, cut = waitingOf(model, p)
	countCut(d, func(t *DetailTruncation) { t.WaitReasons += cut })
	d.Round = roundOf(model, p, d.CapturedLogLines)
}
