package pipeline

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/selfcheck"
	"github.com/abahmed/kwatch/internal/status"
)

// maxCheckedMembers bounds the member entities whose changes are read
// when an incident resolves, so one big storm costs a few reads.
const maxCheckedMembers = 8

// causeStore is the part of a store that keeps the cause self-check
// counts. It is optional so test stores need not implement it.
type causeStore interface {
	LoadCauseChecks() (json.RawMessage, bool, error)
	SaveCauseChecks(json.RawMessage) error
}

// causeHistory is what the check reads from the model.
type causeHistory interface {
	ChangeSets(inventory.EntityID, time.Time) []inventory.ChangeSet
	HealthMarks(inventory.EntityID, time.Time) []inventory.HealthMark
}

// causeChecker records, when an incident resolves, whether recovery
// followed a change to the cause it named. It only counts: nothing it
// learns reaches the solver, the tiers or the messages.
type causeChecker struct {
	model  causeHistory
	counts *selfcheck.Counts
	// save hands the encoded counts to a writer; nil without a store.
	save func(json.RawMessage)
}

func newCauseChecker(model causeHistory) *causeChecker {
	return &causeChecker{model: model, counts: selfcheck.NewCounts()}
}

// restore loads the counts of the previous run.
func (c *causeChecker) restore(store IncidentStore) {
	saved, ok := store.(causeStore)
	if !ok {
		return
	}
	raw, found, err := saved.LoadCauseChecks()
	if err != nil || !found {
		if err != nil {
			klog.ErrorS(err, "pipeline: restore cause checks; starting empty",
				"component", "pipeline", "operation", "restore")
		}
		return
	}
	restored := selfcheck.NewCounts()
	if json.Unmarshal(raw, restored) == nil {
		c.counts = restored
	}
}

// observe counts the incidents these decisions resolved.
func (c *causeChecker) observe(decisions []incident.Decision) {
	changed := false
	for _, d := range decisions {
		if d.Action != incident.Resolve ||
			d.Reason == incident.ReasonSuperseded {
			continue
		}
		inc := d.Incident
		if inc.Cause == nil || inc.Cause.Rule == "" {
			continue
		}
		outcome := selfcheck.Classify(c.evidence(inc))
		c.counts.Record(inc.Cause.Rule, inc.Resolved, outcome)
		changed = true
	}
	if changed && c.save != nil {
		if raw, err := json.Marshal(c.counts); err == nil {
			c.save(raw)
		}
	}
}

// evidence reads the model for the resolved incident inc.
func (c *causeChecker) evidence(inc incident.Incident) selfcheck.Evidence {
	recovered := inc.RecoveringSince
	if recovered.IsZero() {
		recovered = inc.Resolved
	}
	e := selfcheck.Evidence{MembersRecovered: recovered}
	cause := inc.Cause.Root
	members := memberEntities(inc, cause)
	for _, id := range append([]inventory.EntityID{cause}, members...) {
		c.readChanges(&e, id, cause, inc.Opened)
	}
	if slices.Contains(inc.Removed, cause) {
		e.CauseChanged = latest(e.CauseChanged, recovered)
	}
	if !slices.Contains(members, cause) {
		e.CauseRecovered = c.recoveredAt(cause, inc.Opened)
	}
	return e
}

// readChanges adds to e the meaningful changes around id made while the
// incident was open: to the cause itself, or to something else.
func (c *causeChecker) readChanges(
	e *selfcheck.Evidence, id, cause inventory.EntityID, opened time.Time,
) {
	for _, set := range c.model.ChangeSets(id, opened) {
		for _, ch := range set.Changes {
			if !meaningful(ch, opened, e.MembersRecovered) {
				continue
			}
			if ch.Entity == cause {
				e.CauseChanged = latest(e.CauseChanged, ch.At)
			} else {
				e.OtherChanged = latest(e.OtherChanged, ch.At)
			}
		}
	}
}

// memberEntities lists up to maxCheckedMembers entities of the members,
// other than the cause itself, in a stable order.
func memberEntities(
	inc incident.Incident, cause inventory.EntityID,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, f := range inc.Members {
		if f.Entity != cause && !slices.Contains(out, f.Entity) {
			out = append(out, f.Entity)
		}
	}
	slices.SortFunc(out, func(a, b inventory.EntityID) int {
		return strings.Compare(a.String(), b.String())
	})
	if len(out) > maxCheckedMembers {
		out = out[:maxCheckedMembers]
	}
	return out
}

// recoveredAt is when id last stopped failing after since, or zero when
// it was not failing or still is.
func (c *causeChecker) recoveredAt(
	id inventory.EntityID, since time.Time,
) time.Time {
	marks := c.model.HealthMarks(id, since)
	if n := len(marks); n > 0 && !marks[n-1].Failing {
		return marks[n-1].At
	}
	return time.Time{}
}

// meaningful reports a change made while the incident was open that is
// more than the autoscaler reacting to it.
func meaningful(ch inventory.Change, opened, until time.Time) bool {
	return !ch.At.Before(opened) && !ch.At.After(until) &&
		ch.Classify() != inventory.ClassScale
}

func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// report is the self-check as /status shows it.
func (c *causeChecker) report(now time.Time) []status.CauseCheck {
	var out []status.CauseCheck
	for _, r := range c.counts.Summary(now) {
		out = append(out, status.CauseCheck{
			Rule: r.Rule, Confirmed: r.Confirmed, OtherChanged: r.Other,
			NothingChanged: r.None, Total: r.Total(), Text: r.Sentence(),
		})
	}
	return out
}
