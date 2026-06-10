package filter

import (
	"strings"
)

type ContainerKillingFilter struct{}

func (f ContainerKillingFilter) Enrich(ctx *Context) bool {
	if !ctx.Config.IgnoreFailedGracefulShutdown || ctx.Events == nil {
		return false
	}
	container := ctx.Container.Container
	if container.State.Waiting != nil {
		return false
	}
	for _, ev := range *ctx.Events {
		// Graceful shutdown did not work and container was killed during
		// shutdown. Not really an error
		if ev.Reason == "Killing" &&
			strings.Contains(ev.Message, "Stopping container "+container.Name) {
			return true
		}
	}
	return false
}

func (f ContainerKillingFilter) Execute(ctx *Context) bool {
	return f.Enrich(ctx)
}
