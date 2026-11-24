package app

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// linkSink is the health server as dashboard link reporting sees it.
type linkSink interface {
	SetDashboardLinks(health.LinkStatus)
}

// dashboardLinks finds the dashboard address messages link to and says
// why there is none: without it, links vanish from alerts silently. A
// change is logged once and published to /status.
type dashboardLinks struct {
	target dashboardTarget
	model  inventory.Reader
	seen   schemeSource
	// forbidden reports that kwatch may not read Ingresses or
	// HTTPRoutes; nil when unknown.
	forbidden func() bool
	sink      linkSink
	// settled reports that the routes the address comes from were
	// listed; nil when always. wait bounds the first messages' wait
	// for them, linkWait when zero (dashboard_links_wait.go).
	settled  func() bool
	wait     time.Duration
	waitOnce sync.Once
	waited   atomic.Bool

	mu   sync.Mutex
	last health.LinkStatus
	// known is set once a state was logged.
	known bool
}

// address is the dashboard address now, "" when there is none. The
// first call waits for the routes to be listed (awaitRoutes).
func (l *dashboardLinks) address() string {
	l.awaitRoutes()
	return l.Status().Address
}

// Status finds the address now and reports its state, logging and
// publishing a change.
func (l *dashboardLinks) Status() health.LinkStatus {
	address, reason := l.target.lookup(l.model, l.seen)
	if address == "" && l.forbidden != nil && l.forbidden() {
		reason = health.LinksRoutesForbidden
	}
	now := health.LinkStatus{
		On: address != "", Address: address, Reason: reason,
	}
	if !now.On && !l.routesKnown() {
		// Not listed yet: off is not known, so it is not reported.
		return now
	}
	l.mu.Lock()
	changed := !l.known || now != l.last
	l.last, l.known = now, true
	l.mu.Unlock()
	if changed {
		l.report(now)
	}
	return now
}

// report logs and publishes a new state.
func (l *dashboardLinks) report(now health.LinkStatus) {
	if now.On {
		klog.InfoS("dashboard links on", "component", "dashboard",
			"address", now.Address)
	} else {
		klog.InfoS("dashboard links off: messages carry no dashboard "+
			"link", "component", "dashboard", "reason", now.Reason,
			"namespace", l.target.namespace, "service", l.target.service)
	}
	if l.sink != nil {
		l.sink.SetDashboardLinks(now)
	}
}

// routesForbidden reports that the API refuses kwatch the Ingresses or
// the HTTPRoutes, typed or discovered.
func routesForbidden(
	typed sourceAvailability, dynamic dynamicCoverage,
) bool {
	if typed != nil {
		for _, s := range typed.Unavailable() {
			if s.Reason == kube.ReasonPermissionDenied &&
				isRouteResource(s.Resource) {
				return true
			}
		}
	}
	if dynamic == nil {
		return false
	}
	for _, k := range dynamic.Status().Kinds {
		resource, _, _ := strings.Cut(k.Resource, ".")
		if k.Reason == kube.ReasonPermissionDenied &&
			isRouteResource(resource) {
			return true
		}
	}
	return false
}

func isRouteResource(resource string) bool {
	return resource == "ingresses" || resource == "httproutes"
}
