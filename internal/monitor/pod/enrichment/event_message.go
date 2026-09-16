package enrichment

import (
	"github.com/abahmed/kwatch/internal/filter"
)

// EventMessageEnricher suppresses an incident when an attached Kubernetes Event
// contains one of the configured message substrings.
type EventMessageEnricher struct{}

func (enricher EventMessageEnricher) Enrich(ctx *Context) bool {
	if ctx == nil || ctx.Events == nil {
		return false
	}
	for _, ev := range *ctx.Events {
		if filter.MatchesEventMessage(
			ctx.Runtime.Scope().SuppressionIndex(),
			ev.Message,
		) {
			return true
		}
	}
	return false
}
