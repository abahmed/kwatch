package app

import (
	"context"
	"time"

	"github.com/abahmed/kwatch/internal/health"
)

// dashboardResources shows kwatch's own memory, CPU and disk use on the
// dashboard's kwatch page, read from its container's cgroup and the
// state file's volume. It samples the CPU every health.SelfUseEvery.
func (p *pipelineBuild) dashboardResources() func(context.Context) {
	dir := ""
	if p.state != nil {
		dir = dataDir()
	}
	reader := health.NewUsageReader(health.CgroupRoot, dir,
		p.deps.clients.Clock.Now)
	dash := p.deps.dashboardServer
	return func(ctx context.Context) {
		dash.SetResources(reader.Use)
		defer dash.SetResources(nil)
		tick := time.NewTicker(health.SelfUseEvery)
		defer tick.Stop()
		reader.Run(ctx, tick.C)
	}
}
