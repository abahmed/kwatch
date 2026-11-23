package scenarios

import (
	"fmt"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
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
	// bootHeld is set when the node boot grace holds the message on
	// purpose, so it has its own gate.
	bootHeld bool
	// episodeHeld is set when the message waits on purpose for a
	// repeating failure to show its pattern, so it has its own gate.
	episodeHeld bool
}

// episodeHeld lists the scenarios whose first message waits for the
// repeats that make a pattern: a service that goes down and comes back
// three times in a quarter of an hour. One blip is not a failure, so
// the pattern is the proof and the third outage is when it exists.
var episodeHeld = map[string]bool{
	"etcd-path-intermittent-during-maintenance": true,
}

// episodeHold is the longest an episode-held message may take: the
// window in which the repeats are counted, and the usual allowance.
const episodeHold = 15 * time.Minute

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
		start := firstObservation(log, p, failureStart(log, p, at))
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

func failureStart(
	log replay.Log, p incident.Incident, announced time.Time,
) time.Time {
	floor := log.Start
	start, symptom := p.Opened, time.Time{}
	var later time.Time
	for _, finding := range p.Members {
		// A configuration risk predates the failure and is not one.
		// Replicas that run different builds of one tag are healthy
		// side by side until one of them fails: the outage begins
		// then, not when the builds began to differ.
		if finding.Advisory || finding.Reason == reasons.ImageDigestDrift {
			continue
		}
		if finding.Since.IsZero() || finding.Since.After(announced) {
			continue
		}
		if finding.Since.Before(start) {
			start = finding.Since
		}
		if finding.Since.After(floor) && (later.IsZero() ||
			finding.Since.Before(later)) {
			later = finding.Since
		}
		if finding.Entity != p.Root && (symptom.IsZero() ||
			finding.Since.Before(symptom)) {
			symptom = finding.Since
		}
	}
	// A standing condition (a cordon, a full quota, a stuck finalizer)
	// predates what it breaks and is not an outage by itself: the
	// failure begins with the first symptom the incident reports, the
	// first pod or request that went wrong, when one follows it.
	if !symptom.IsZero() && symptom.After(start) {
		start = symptom
	}
	if !start.After(floor) {
		// The condition was already there when kwatch began to watch,
		// so its onset was not observed. The failure begins with the
		// first symptom that follows: a member that went wrong after
		// the log began, or else the first change the cluster made.
		switch {
		case !later.IsZero():
			return later
		default:
			return firstChange(log, floor, announced)
		}
	}
	return start
}

// firstChange is the time of the first entry after since that reports a
// change or a removal, up to until; since when there is none.
func firstChange(log replay.Log, since, until time.Time) time.Time {
	for _, entry := range log.Entries {
		if entry.At.After(since) && !entry.At.After(until) &&
			(entry.Observation.Kind == inventory.Changed ||
				entry.Observation.Kind == inventory.Gone) {
			return entry.At
		}
	}
	return since
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
// report its 95th percentile next to it. Messages held on purpose by
// the node boot grace are left out of those and have a gate of their
// own: the boot window plus the same allowance.
func firstMessageGates(measured []firstMessage) []scorecard.Gate {
	limits := []struct {
		tier  incident.Tier
		limit time.Duration
	}{
		{incident.Page, scorecard.GoalPageFirstMessage},
		{incident.Notify, scorecard.GoalNotifyFirstMessage},
	}
	gates := make([]scorecard.Gate, 0, len(limits)+1)
	for _, l := range limits {
		var delays []time.Duration
		for _, m := range measured {
			if !m.bootHeld && !m.episodeHeld && m.tier == l.tier {
				delays = append(delays, m.delay)
			}
		}
		gates = append(gates, firstMessageGate(l.tier, delays, l.limit))
	}
	var held []time.Duration
	for _, m := range measured {
		if m.bootHeld {
			held = append(held, m.delay)
		}
	}
	var episodes []time.Duration
	for _, m := range measured {
		if m.episodeHeld {
			episodes = append(episodes, m.delay)
		}
	}
	gates = append(gates, namedFirstMessageGate(
		"Time to first message (boot-held)", held,
		kube.BootWindow+scorecard.GoalNotifyFirstMessage))
	return append(gates, namedFirstMessageGate(
		"Time to first message (episode-held)", episodes, episodeHold))
}

func firstMessageGate(
	tier incident.Tier, delays []time.Duration, limit time.Duration,
) scorecard.Gate {
	return namedFirstMessageGate(
		"Time to first message ("+tier.String()+" tier)", delays, limit)
}

func namedFirstMessageGate(
	name string, delays []time.Duration, limit time.Duration,
) scorecard.Gate {
	gate := scorecard.Gate{
		Name:   name,
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
