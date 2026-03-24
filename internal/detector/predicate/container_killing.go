package predicate

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detector"
)

// ContainerKilling filters out graceful shutdown failures
type ContainerKilling struct {
	IgnoreFailedGracefulShutdown bool
}

func NewContainerKilling(ignore bool) *ContainerKilling {
	return &ContainerKilling{
		IgnoreFailedGracefulShutdown: ignore,
	}
}

func (c *ContainerKilling) Name() string {
	return "ContainerKillingPredicate"
}

func (c *ContainerKilling) Filter(input *detector.Input) bool {
	if !c.IgnoreFailedGracefulShutdown || input.Events == nil {
		return false
	}

	for _, ev := range *input.Events {
		if ev.Reason == "Killing" && strings.Contains(ev.Message, "Stopping container") {
			return true // filter out
		}
	}

	return false
}
