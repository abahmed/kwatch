package app

import (
	"context"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// dashboardPush runs the dashboard's browser notifications with the
// pipeline, whose store keeps the push key and the browsers. They go
// out through the same guarded client as every provider, so they honor
// the proxy and trusted CAs and never reach a cloud metadata address.
func (p *pipelineBuild) dashboardPush() func(context.Context) {
	client := transport.GuardMetadata(p.deps.clients.HTTP)
	send := transport.NewSender(transport.Dependencies{
		HTTPClient: client, Clock: p.deps.clients.Clock,
	})
	dash := p.deps.dashboardServer
	return func(ctx context.Context) {
		dash.RunPush(ctx, p.state, send)
	}
}
