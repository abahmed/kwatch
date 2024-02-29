package filter

import (
	"strings"
)

type ContainerKillingFilter struct{}

func (f ContainerKillingFilter) Execute(ctx *Context) bool {
	if !ctx.Config.IgnoreFailedGracefulShutdown || ctx.Events == nil {
		return false
	}
	container := ctx.Container.Container

	isOk := false
	if container.State.Waiting != nil {
		return isOk
	}

	for _, ev := range *ctx.Events {
		// Graceful shutdown did not work and container was killed during
		// shutdown. Not really an error
		if ev.Reason == "Killing" &&
			strings.Contains(
				ev.Message,
				"Stopping container "+container.Name) {
			isOk = true
		}
	}

	return isOk
}
