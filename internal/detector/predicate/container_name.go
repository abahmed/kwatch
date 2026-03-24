package predicate

import (
	"slices"

	"github.com/abahmed/kwatch/internal/detector"
)

// ContainerName filters events by container name
type ContainerName struct {
	Allowed   []string
	Forbidden []string
}

func NewContainerName(allowed, forbidden []string) *ContainerName {
	return &ContainerName{
		Allowed:   allowed,
		Forbidden: forbidden,
	}
}

func (c *ContainerName) Name() string {
	return "ContainerNamePredicate"
}

func (c *ContainerName) Filter(input *detector.Input) bool {
	if input.Container == nil {
		return false
	}

	containerName := input.Container.Name

	if len(c.Allowed) > 0 && !slices.Contains(c.Allowed, containerName) {
		return true // filter out
	}

	if len(c.Forbidden) > 0 && slices.Contains(c.Forbidden, containerName) {
		return true // filter out
	}

	return false
}
