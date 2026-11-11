package app

import (
	"context"

	"k8s.io/klog/v2"
)

const (
	dashboardComponent = "dashboard-server"
	// dashboardCredentialsComponent is on /health only while the
	// dashboard's credentials are missing.
	dashboardCredentialsComponent = "dashboard-credentials"
)

// openDashboardEarly binds the dashboard port while the generation is
// built. The dashboard is optional and not required for readiness: a
// failure does not stop kwatch, and the listener is tried again while
// kwatch runs (startDashboard).
func openDashboardEarly(ctx context.Context, deps *serverDeps) {
	if deps.dashboardServer == nil {
		return
	}
	if err := deps.dashboardServer.Open(ctx); err != nil {
		klog.ErrorS(err, "failed to open dashboard listener; it is "+
			"tried again while kwatch runs",
			"component", dashboardComponent)
	}
}

// startDashboard serves the dashboard under the supervisor's restart
// loop. A listener that cannot open or serve (a taken port, a TLS key
// pair that cannot be read) shows as degraded on /health and is tried
// again with backoff, so it comes back once the cause is fixed, without
// a pod restart. Credentials that could not be read at start are tried
// again the same way.
func startDashboard(
	ctx context.Context, deps *serverDeps, supervisor *componentSupervisor,
) {
	if deps.dashboardServer == nil {
		return
	}
	supervisor.startOptional(ctx, componentSpec{
		name:    dashboardComponent,
		onError: degrade(deps, dashboardComponent),
		run: func(ctx context.Context) error {
			return runDashboard(ctx, deps)
		},
	})
	if !dashboardCredentialsMissing(deps) {
		return
	}
	deps.healthServer.SetComponentStatus(dashboardCredentialsComponent,
		"degraded", "component_failed", false)
	supervisor.startOptional(ctx, componentSpec{
		name: dashboardCredentialsComponent,
		// It is done once the credentials are read.
		cleanStop: true,
		onError:   degrade(deps, dashboardCredentialsComponent),
		run: func(ctx context.Context) error {
			return loadDashboardCredentials(ctx, deps)
		},
	})
}

// runDashboard opens the listener if it is not open and serves it.
func runDashboard(ctx context.Context, deps *serverDeps) error {
	if err := deps.dashboardServer.Open(ctx); err != nil {
		if ctx.Err() != nil {
			// Stopped for shutdown: nothing to report.
			return nil
		}
		return err
	}
	recoverComponent(deps, dashboardComponent)()
	return deps.dashboardServer.Serve(ctx)
}

// dashboardCredentialsMissing reports a chart-managed Secret that could
// not be read when the server was built: kwatch fills that Secret
// itself, so empty credentials mean the read failed. Credentials an
// operator left empty in the environment are a choice, not a failure.
func dashboardCredentialsMissing(deps *serverDeps) bool {
	return dashboardSecretName() != "" && deps.clients.Kubernetes != nil &&
		!deps.dashboardServer.HasCredentials()
}

// loadDashboardCredentials reads the credentials again and hands them to
// the running server. An error makes the supervisor try again later.
func loadDashboardCredentials(ctx context.Context, deps *serverDeps) error {
	token, password, err := readDashboardCredentials(
		ctx, deps.clients.Kubernetes, envOrKwatch("POD_NAMESPACE"))
	if err != nil {
		return err
	}
	deps.dashboardServer.SetCredentials(token, password)
	deps.healthServer.ClearComponentStatus(dashboardCredentialsComponent)
	klog.InfoS("dashboard credentials read; sign-in works now",
		"component", dashboardComponent)
	return nil
}
