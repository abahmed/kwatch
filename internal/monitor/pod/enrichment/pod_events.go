package enrichment

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

type PodEventsEnricher struct{}

func (enricher PodEventsEnricher) Enrich(ctx *Context) bool {
	if !ctx.PodHasIssues || ctx.Events == nil {
		return false
	}
	for _, ev := range *ctx.Events {
		if ev.Type == corev1.EventTypeWarning && strings.Contains(ev.Message, "deleting pod") {
			ctx.PodHasIssues = false
			ctx.ContainersHasIssues = false
			return true
		}
	}
	return false
}
