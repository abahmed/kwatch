package filter

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

type PodEventsFilter struct{}

func (f PodEventsFilter) Execute(ctx *Context) bool {
	if !ctx.PodHasIssues {
		return false
	}

	if ctx.Events == nil {
		return false
	}

	for _, ev := range *ctx.Events {
		if ev.Type == corev1.EventTypeWarning {
			if strings.Contains(ev.Message, "deleting pod") {
				ctx.PodHasIssues = false
				ctx.ContainersHasIssues = false
				return true
			}
		}
	}
	return false
}
