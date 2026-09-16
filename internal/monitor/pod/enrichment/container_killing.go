package enrichment

import (
	"strings"
)

type ContainerKillingEnricher struct{}

func (enricher ContainerKillingEnricher) Enrich(ctx *Context) bool {
	if !ctx.Runtime.Monitors().IgnoreGracefulKill() ||
		ctx.Events == nil || ctx.Container == nil {
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
			strings.TrimSpace(ev.Message) == "Stopping container "+container.Name {
			return true
		}
	}
	return false
}
