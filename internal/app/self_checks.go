package app

import (
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/health"
)

// crashLogsEnabled reports whether crashed containers' previous logs
// are read: only with investigation, and only while its log reads
// (investigation.logs) are on.
func crashLogsEnabled(deps *serverDeps) bool {
	return deps.has(feature.Investigation) &&
		deps.runtime.Application().InvestigationLogs
}

// podLogsReader is the crash-log round as health reporting sees it.
type podLogsReader interface {
	PodLogsForbidden() bool
}

// reportPodLogs publishes the pod-logs component: degraded while the
// API refuses kwatch the pod logs, so crash output is not shown. It is
// a permission gap: health only, never an incident. The health server
// records only transitions, so it may run on every report.
func reportPodLogs(sink sourceStatusSink, logs podLogsReader) {
	if sink == nil || logs == nil {
		return
	}
	if logs.PodLogsForbidden() {
		sink.SetComponentStatus(health.PodLogsComponent, "degraded",
			"optional_permission_denied", false)
		return
	}
	sink.SetComponentStatus(health.PodLogsComponent, "running", "", true)
}

// withSelfChecks adds to the source health reports the coverage facts
// that are not watches: pod log access and the dashboard links. Both
// are read through p when the report runs, since the crash-log round
// is built after the source health.
func (p *pipelineBuild) withSelfChecks(h *sourceHealth) *sourceHealth {
	sink := healthSink(p.deps)
	return h.withCheck(func() {
		if p.crashLogs != nil {
			reportPodLogs(sink, p.crashLogs)
		}
		if p.links != nil {
			p.links.Status()
		}
	})
}

// podLogsForbidden reports whether pod log reads are refused; false
// when they are not made.
func (p *pipelineBuild) podLogsForbidden() bool {
	return p.crashLogs != nil && p.crashLogs.PodLogsForbidden()
}

// routesForbidden reports that kwatch may not read the Ingresses or
// HTTPRoutes its dashboard address comes from.
func (p *pipelineBuild) routesForbidden() bool {
	if p.source == nil {
		return false
	}
	var dynamic dynamicCoverage
	if p.dynamic != nil {
		dynamic = p.dynamic
	}
	return routesForbidden(p.source, dynamic)
}

// linkSinkOf is the health server as a link sink, or nil without one
// (a nil pointer must not become a non-nil interface).
func linkSinkOf(deps *serverDeps) linkSink {
	if deps.healthServer == nil {
		return nil
	}
	return deps.healthServer
}
