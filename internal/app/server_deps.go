package app

import (
	"context"

	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controller"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/pvc"
)

// serverDeps contains the components owned by the application runtime. The
// grouping makes ownership and shutdown ordering visible without hiding the
// composition wiring in a generic component registry.
type serverDeps struct {
	ctx                  context.Context
	cancel               context.CancelFunc
	runtime              config.RuntimeConfig
	clients              client.ClientSet
	healthServer         *health.HealthServer
	deliveryManager      *delivery.Manager
	incidentEngine       *incident.Engine
	pvcMonitor           *pvc.PvcMonitor
	hbMonitor            *heartbeat.HeartbeatMonitor
	ctl                  *controller.Controller
	incidentCh           chan stateSnapshot
	incidentSaver        incidentSaver
	incidentDone         <-chan struct{}
	baselineDone         <-chan struct{}
	changeDone           <-chan struct{}
	feedbackDone         <-chan struct{}
	startPersistence     func(context.Context, *componentSupervisor)
	initialized          <-chan struct{}
	controllerDone       chan struct{}
	notifyStartup        func()
	notifyStartupSummary func(map[string]int)
	// recordAlive stamps the liveness marker that lets the next start report
	// how long monitoring was down.
	recordAlive     func(context.Context)
	closeAudit      func() error
	cleanup         func()
	tlsSweep        func() error
	statusRun       func(context.Context) error
	metricsRun      func(context.Context) error
	probeRun        func(context.Context) error
	kubeletRun      func(context.Context) error
	storageRun      func(context.Context) error
	networkRun      func(context.Context) error
	securityRun     func(context.Context) error
	controlPlaneRun func(context.Context) error
	telemetryRun    func(context.Context) error
	upgradeRun      func(context.Context) error
}

// componentSpec names a background component and records whether its failure
// is fatal. The application owns the goroutine and observes the returned
// error; components keep their own domain-specific stop behavior.
type componentSpec struct {
	name     string
	required bool
	run      func(context.Context) error
	onError  func(error)
}
