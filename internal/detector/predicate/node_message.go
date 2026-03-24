package predicate

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detector"
)

// NodeMessage filters node events by message patterns to ignore
type NodeMessage struct {
	IgnoreMessages []string
}

func NewNodeMessage(ignoreMessages []string) *NodeMessage {
	return &NodeMessage{
		IgnoreMessages: ignoreMessages,
	}
}

func (n *NodeMessage) Name() string {
	return "NodeMessagePredicate"
}

func (n *NodeMessage) Filter(input *detector.Input) bool {
	if input.Node == nil || input.Message == "" {
		return false
	}

	for _, msg := range n.IgnoreMessages {
		if strings.Contains(input.Message, msg) {
			return true // filter out
		}
	}

	return false
}
