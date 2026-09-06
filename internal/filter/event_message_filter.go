package filter

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// EventMessageFilter suppresses an incident when an attached Kubernetes Event
// contains one of the configured message substrings.
type EventMessageFilter struct{}

func (f EventMessageFilter) Enrich(ctx *Context) bool {
	if ctx == nil || ctx.Config == nil || ctx.Events == nil {
		return false
	}
	for _, ev := range *ctx.Events {
		if matchesEventMessage(ev, ctx.Config.Suppression.EventMessages) {
			return true
		}
	}
	return false
}

func (f EventMessageFilter) Execute(ctx *Context) bool {
	return f.Enrich(ctx)
}

func matchesEventMessage(ev corev1.Event, messages []string) bool {
	if ev.Message == "" {
		return false
	}
	for _, message := range messages {
		if message != "" && strings.Contains(ev.Message, message) {
			return true
		}
	}
	return false
}
