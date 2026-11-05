package app

import (
	"context"
	"os"

	"github.com/abahmed/kwatch/internal/dashboard"
	"github.com/abahmed/kwatch/internal/incident"
)

// dashboardReleases keeps the release book of the dashboard's Releases
// page with the pipeline, in the state file when there is one. kwatch's
// own Deployment (the chart's POD_NAMESPACE and POD_NAME) is left out:
// its rollouts are kwatch restarting.
func (p *pipelineBuild) dashboardReleases() func(context.Context) {
	dash := p.deps.dashboardServer
	own := dashboard.OwnWorkload{Namespace: envOrKwatch("POD_NAMESPACE"),
		Name: incident.OwnWorkloadOf(os.Getenv("POD_NAME"))}
	return func(ctx context.Context) {
		dash.RunReleases(ctx, p.state, own)
	}
}
