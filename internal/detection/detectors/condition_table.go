package detectors

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// messageMode says whether a finding quotes the condition's message.
type messageMode uint8

// The zero value adds no evidence.
const (
	// quoteMessage adds the message, even when it is empty.
	quoteMessage messageMode = iota + 1
	// quoteMessageIfAny adds the message only when there is one.
	quoteMessageIfAny
)

// conditionCheck is one row of the condition table: a status condition
// of one kind that, once it has held for its grace, is a finding. The
// summary may use {type}, {reason} and {reasonNote} (" (reason)", or
// nothing when the condition has no reason).
type conditionCheck struct {
	cond string
	// status and reason are the values that mean trouble; empty matches
	// any.
	status string
	reason string
	// grace is how long the condition must hold; zero reports at once.
	grace time.Duration
	// key names the wait, so two bad states never share a first sighting.
	key      string
	finding  string
	severity detection.Severity
	// health is left to Classify when zero.
	health  detection.Health
	summary string
	message messageMode
	// group lists checks of which only the first that fires reports.
	group string
}

// kindConditions maps a kind to its condition checks, in report order.
// Detectors that fold into the table call conditionFindings at the place
// they used to report these conditions.
var kindConditions = map[inventory.Kind][]conditionCheck{}

// addConditions registers checks for kinds.
func addConditions(kinds []inventory.Kind, checks ...conditionCheck) {
	for _, kind := range kinds {
		kindConditions[kind] = append(kindConditions[kind], checks...)
	}
}

// conditionFindings evaluates the table rows of the entity's kind.
func conditionFindings(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	var out []detection.Finding
	reported := map[string]bool{}
	for _, check := range kindConditions[e.ID.Kind] {
		if check.group != "" && reported[check.group] {
			continue
		}
		f, ok := check.evaluate(ctx, e)
		if !ok {
			continue
		}
		reported[check.group] = true
		out = append(out, f)
	}
	return out
}

// evaluate reports the check's finding when its condition is in the bad
// state and has held for its grace.
func (c conditionCheck) evaluate(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	status, reason, since := condition(e, c.cond)
	if (c.status != "" && status != c.status) ||
		(c.reason != "" && reason != c.reason) {
		return detection.Finding{}, false
	}
	if c.grace > 0 && !sustained(ctx, c.key, since, c.grace) {
		return detection.Finding{}, false
	}
	f := detection.Finding{
		Reason: c.finding, Severity: c.severity, Health: c.health,
		Since: since, Summary: c.words(reason),
	}
	message := conditionMessage(e, c.cond)
	if c.message == quoteMessage ||
		(c.message == quoteMessageIfAny && message != "") {
		f.Evidence = []detection.Evidence{
			{Label: "message", Value: message}}
	}
	return f, true
}

// words fills the placeholders of the summary.
func (c conditionCheck) words(reason string) string {
	note := ""
	if reason != "" {
		note = " (" + reason + ")"
	}
	return strings.NewReplacer("{type}", c.cond, "{reason}", reason,
		"{reasonNote}", note).Replace(c.summary)
}
