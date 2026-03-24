package predicate

import (
	"regexp"

	"github.com/abahmed/kwatch/internal/detector"
)

// PodName filters events by pod name regex patterns
type PodName struct {
	Patterns []*regexp.Regexp
}

func NewPodName(patterns []*regexp.Regexp) *PodName {
	return &PodName{
		Patterns: patterns,
	}
}

func (p *PodName) Name() string {
	return "PodNamePredicate"
}

func (p *PodName) Filter(input *detector.Input) bool {
	if input.Pod == nil {
		return false
	}

	podName := input.Pod.Name

	for _, pattern := range p.Patterns {
		if pattern.MatchString(podName) {
			return true // filter out
		}
	}

	return false
}
