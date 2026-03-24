package predicate

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detector"
)

// PodEvents filters out "deleting pod" events
type PodEvents struct{}

func NewPodEvents() *PodEvents {
	return &PodEvents{}
}

func (p *PodEvents) Name() string {
	return "PodEventsPredicate"
}

func (p *PodEvents) Filter(input *detector.Input) bool {
	if input.Events == nil {
		return false
	}

	for _, ev := range *input.Events {
		if ev.Reason == "Scheduled" && strings.Contains(ev.Message, "Pod is marked as") && strings.Contains(ev.Message, "deleting") {
			return true // filter out
		}
	}

	return false
}
