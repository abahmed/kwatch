package filter

import (
	"strings"

	"github.com/abahmed/kwatch/util"
	corev1 "k8s.io/api/core/v1"
)

type PodEventsFilter struct{}

func (f PodEventsFilter) Execute(ctx *Context) bool {
	if !ctx.PodHasIssues {
		return false
	}
	events, _ := util.GetPodEvents(ctx.Client, ctx.Pod.Name, ctx.Pod.Namespace)
	ctx.Events = &events.Items

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
