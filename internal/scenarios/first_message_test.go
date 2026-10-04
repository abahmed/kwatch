package scenarios

import (
	"fmt"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/scorecard"
)

// firstMessage is how long one scenario took to interrupt people: from
// the first failure observation to the first notify or page message, on
// the simulated clock.
type firstMessage struct {
	scenario string
	tier     incident.Tier
	delay    time.Duration
	// at is when the message went, on the simulated clock.
	at time.Time
}

// timeToFirstMessage measures one replay. It reports false when nothing
// interrupted people: a quiet scenario, or one whose incidents are only
// digests.
//
// The failure is first observed when kwatch receives the first
// observation of an entity the incident explains (a member finding's
// entity or the root) at or after the condition began. The condition
// begins at the earliest Since of those findings: when Kubernetes says
// it began, or when kwatch first detected it. It is never earlier than
// the log's start, because fixtures date healthy history before it.
func timeToFirstMessage(
	name string, log replay.Log, result replay.Result,
) (firstMessage, bool) {
	var first firstMessage
	found := false
	consider := func(d incident.Decision, at time.Time) {
		p := d.Incident
		if d.Action != incident.Announce || p.ID == "" ||
			p.Tier < incident.Notify {
			return
		}
		start := firstObservation(log, p, failureStart(p, log.Start))
		if candidate := (firstMessage{scenario: name, tier: p.Tier,
			delay: at.Sub(start)}); !found || at.Before(first.at) {
			first, first.at, found = candidate, at, true
		}
	}
	for i, d := range result.Decisions {
		consider(d, result.Times[i])
	}
	// A roll-up interrupts people the moment its announcements are made.
	for _, c := range result.Carried {
		if c.Carrier == "roll-up" {
			consider(c.Decision, c.At)
		}
	}
	return first, found
}

func failureStart(p incident.Incident, floor time.Time) time.Time {
	start := p.Opened
	for _, finding := range p.Members {
		// A configuration risk predates the failure and is not one.
		if finding.Advisory {
			continue
		}
		if !finding.Since.IsZero() && finding.Since.Before(start) {
			start = finding.Since
		}
	}
	if start.Before(floor) {
		return floor
	}
	return start
}

// firstObservation is the time of the first log entry about an entity
// of p at or after since; since when there is none.
func firstObservation(
	log replay.Log, p incident.Incident, since time.Time,
) time.Time {
	involved := map[inventory.EntityID]bool{p.Root: true}
	for key := range p.Members {
		involved[key.Entity] = true
	}
	for _, entry := range log.Entries {
		if !entry.At.Before(since) && involved[entry.Observation.Entity] {
			return entry.At
		}
	}
	return since
}

// firstMessageGates gate the slowest first message of each tier and
// report its 95th percentile next to it.
func firstMessageGates(measured []firstMessage) []scorecard.Gate {
	limits := []struct {
		tier  incident.Tier
		limit time.Duration
	}{
		{incident.Page, scorecard.GoalPageFirstMessage},
		{incident.Notify, scorecard.GoalNotifyFirstMessage},
	}
	gates := make([]scorecard.Gate, 0, len(limits))
	for _, l := range limits {
		var delays []time.Duration
		for _, m := range measured {
			if m.tier == l.tier {
				delays = append(delays, m.delay)
			}
		}
		gates = append(gates, firstMessageGate(l.tier, delays, l.limit))
	}
	return gates
}

func firstMessageGate(
	tier incident.Tier, delays []time.Duration, limit time.Duration,
) scorecard.Gate {
	gate := scorecard.Gate{
		Name:   "Time to first message (" + tier.String() + " tier)",
		Target: "max <= " + limit.String(),
		Value:  "no cases",
		Pass:   true,
	}
	if len(delays) == 0 {
		return gate
	}
	sort.Slice(delays, func(i, j int) bool { return delays[i] < delays[j] })
	p95 := delays[(len(delays)*95+99)/100-1]
	slowest := delays[len(delays)-1]
	gate.Value = fmt.Sprintf("p95 %s, max %s (%d scenarios)", p95,
		slowest, len(delays))
	gate.Pass = slowest <= limit
	return gate
}
