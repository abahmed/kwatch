package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func text(e inventory.Entity, name string) string {
	attribute, ok := e.Attribute(name)
	if !ok {
		return ""
	}
	return attribute.Value.AsText()
}

func number(e inventory.Entity, name string) (float64, bool) {
	attribute, ok := e.Attribute(name)
	if !ok {
		return 0, false
	}
	return attribute.Value.AsNumber()
}

func flag(e inventory.Entity, name string) bool {
	attribute, ok := e.Attribute(name)
	if !ok {
		return false
	}
	value, _ := attribute.Value.AsBool()
	return value
}

func timestamp(e inventory.Entity, name string) time.Time {
	attribute, ok := e.Attribute(name)
	if !ok {
		return time.Time{}
	}
	return attribute.Value.AsTime()
}

// valueSince is when the attribute took its current value.
func valueSince(e inventory.Entity, name string) time.Time {
	attribute, ok := e.Attribute(name)
	if !ok {
		return time.Time{}
	}
	return attribute.Since
}

// condition returns a condition's status, reason and transition time.
func condition(
	e inventory.Entity, conditionType string,
) (status, reason string, since time.Time) {
	key := kube.ConditionKey(conditionType)
	return text(e, key), text(e, key+kube.AttrConditionReason),
		timestamp(e, key+kube.AttrConditionSince)
}

// sustained reports whether a condition that started at since has lasted
// for threshold. When it has not, it asks for a recheck at the deadline.
// An unknown start (zero since) does not skip the wait: the condition
// counts from when it was first seen, remembered across evaluations
// under key. Each bad state needs its own key, so that two states of
// one entity never share (and inherit) one another's first sighting.
func sustained(
	ctx detection.Context, key string, since time.Time,
	threshold time.Duration,
) bool {
	if since.IsZero() {
		since = ctx.Onset("sustained/"+key, ctx.Now)
	}
	remaining := threshold - ctx.Now.Sub(since)
	if remaining <= 0 {
		return true
	}
	ctx.RecheckAfter(remaining)
	return false
}
