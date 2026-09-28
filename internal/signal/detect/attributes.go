package detect

import (
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

func text(e knowledge.Entity, name string) string {
	attribute, ok := e.Attribute(name)
	if !ok {
		return ""
	}
	return attribute.Value.AsText()
}

func number(e knowledge.Entity, name string) (float64, bool) {
	attribute, ok := e.Attribute(name)
	if !ok {
		return 0, false
	}
	return attribute.Value.AsNumber()
}

func flag(e knowledge.Entity, name string) bool {
	attribute, ok := e.Attribute(name)
	if !ok {
		return false
	}
	value, _ := attribute.Value.AsBool()
	return value
}

func timestamp(e knowledge.Entity, name string) time.Time {
	attribute, ok := e.Attribute(name)
	if !ok {
		return time.Time{}
	}
	return attribute.Value.AsTime()
}

// valueSince is when the attribute took its current value.
func valueSince(e knowledge.Entity, name string) time.Time {
	attribute, ok := e.Attribute(name)
	if !ok {
		return time.Time{}
	}
	return attribute.Since
}

// condition returns a condition's status, reason and transition time.
func condition(
	e knowledge.Entity, conditionType string,
) (status, reason string, since time.Time) {
	key := kube.ConditionKey(conditionType)
	return text(e, key), text(e, key+kube.AttrConditionReason),
		timestamp(e, key+kube.AttrConditionSince)
}

// sustained reports whether a condition that started at since has lasted
// for threshold. When it has not, it asks for a recheck at the deadline.
func sustained(
	ctx signal.Context, since time.Time, threshold time.Duration,
) bool {
	if since.IsZero() {
		return true
	}
	remaining := threshold - ctx.Now.Sub(since)
	if remaining <= 0 {
		return true
	}
	ctx.RecheckAfter(remaining)
	return false
}
