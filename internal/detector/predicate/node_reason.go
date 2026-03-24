package predicate

import (
	"github.com/abahmed/kwatch/internal/detector"
)

// NodeReason filters node events by reason to ignore
type NodeReason struct {
	IgnoreReasons []string
}

func NewNodeReason(ignoreReasons []string) *NodeReason {
	return &NodeReason{
		IgnoreReasons: ignoreReasons,
	}
}

func (n *NodeReason) Name() string {
	return "NodeReasonPredicate"
}

func (n *NodeReason) Filter(input *detector.Input) bool {
	if input.Node == nil || input.Reason == "" {
		return false
	}

	for _, reason := range n.IgnoreReasons {
		if input.Reason == reason {
			return true // filter out
		}
	}

	return false
}
