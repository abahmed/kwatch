package app

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/metrics"
)

// Self alert reasons that are not health reasons: kwatch's own probes.
const (
	selfReasonLag     = "decision_lag"
	selfReasonBacklog = "outbox_backlog"
	// selfLagLimit is the average decision lag, over one check, above
	// which kwatch is falling behind.
	selfLagLimit = 10 * time.Second
)

// selfQuietParts are the parts never told about: leadership (a kwatch
// that lost the lock cannot send), the license and a refused
// KwatchConfig (each has a notice of its own), the coverage summary and
// the permission audit (permission gaps chosen at install), and parts
// whose loss changes nothing that is watched.
var selfQuietParts = map[string]bool{
	"leadership": true, "leader-election": true, "license": true,
	"config-overlay": true, "crd-watcher": true, "coverage": true,
	"rbac": true, "telemetry": true, "upgrader": true,
	"dashboard": true, "dashboard-credentials": true,
	"self-alerts": true,
}

// selfProofParts are the components that report running before they
// have read anything, so running alone proves no access.
var selfProofParts = map[string]bool{health.PodLogsComponent: true}

// selfReportable reports whether a degraded part is worth a message:
// a reason in selfReasonWords on a part that is not quiet. Normal
// states (an API that is not installed, a kind turned off, a sync
// still running, a heartbeat not set up) never are.
func selfReportable(part, reason string) bool {
	if selfQuietParts[part] || strings.HasPrefix(part, "dashboard") {
		return false
	}
	_, ok := selfReasonWords[reason]
	return ok && reason != selfReasonLag && reason != selfReasonBacklog
}

// selfProbes reads the parts of kwatch that are degraded now: the
// health components and two probes of its own, decision lag and a
// queue of unsent alerts that does not drain.
type selfProbes struct {
	components func() map[string]health.ComponentStatus
	lag        *metrics.LagHistogram
	backlog    func() int64
	// accessProven tells whether a part in selfProofParts has had a
	// read succeed; nil means none has.
	accessProven func(part string) bool
	lastCount    uint64
	lastSum      float64
}

// conditions are the degraded parts at now, sorted by nothing: the
// alerter keys them by part.
func (p *selfProbes) conditions(time.Time) []selfCondition {
	var out []selfCondition
	if p.components != nil {
		for part, c := range p.components() {
			if c.State == "degraded" && selfReportable(part, c.Reason) {
				out = append(out, selfCondition{Part: part, Reason: c.Reason})
			}
		}
	}
	if p.lagging() {
		out = append(out, selfCondition{Part: "decision-lag",
			Reason: selfReasonLag})
	}
	if p.backlog != nil && p.backlog() > 0 {
		out = append(out, selfCondition{Part: "delivery-backlog",
			Reason: selfReasonBacklog})
	}
	return out
}

// provenParts are the components seen working now: running, and for
// the ones that run before any read is made, a read that succeeded.
func (p *selfProbes) provenParts() []string {
	var out []string
	if p.components == nil {
		return out
	}
	for part, c := range p.components() {
		if c.State != "running" || !c.Available {
			continue
		}
		if selfProofParts[part] &&
			(p.accessProven == nil || !p.accessProven(part)) {
			continue
		}
		out = append(out, part)
	}
	return out
}

// lagging reports whether the decisions since the last check waited
// longer than selfLagLimit on average; no decision is no lag.
func (p *selfProbes) lagging() bool {
	if p.lag == nil {
		return false
	}
	count, sum := p.lag.Count(), p.lag.Sum()
	n, total := count-p.lastCount, sum-p.lastSum
	p.lastCount, p.lastSum = count, sum
	return n > 0 && total/float64(n) > selfLagLimit.Seconds()
}

// newSelfAlerter wires self alerts for the session, or returns nil
// when they are off (app.disableSelfAlerts) or nothing could send or
// report them. Both editions run them; edits of an open message need
// threads (Pro), so Community gets the opening and the close only.
func newSelfAlerter(deps *serverDeps, store selfAlertStore) *selfAlerter {
	app := deps.runtime.Application()
	if app.DisableSelfAlerts || deps.healthServer == nil ||
		deps.deliveryManager == nil || store == nil {
		return nil
	}
	reg := metrics.DefaultRegistry()
	probes := &selfProbes{components: deps.healthServer.ComponentStatuses,
		accessProven: deps.healthServer.AccessProven,
		lag:          &reg.DecisionLag, backlog: reg.Delivery.OutboxDepth.Load}
	probes.lastCount, probes.lastSum = reg.DecisionLag.Count(),
		reg.DecisionLag.Sum()
	return &selfAlerter{conditions: probes.conditions,
		proven: probes.provenParts,
		sink:   deps.deliveryManager, store: store,
		now: deps.clients.Clock.Now, cluster: deps.clusterName(),
		updates: deps.has(feature.NativeThreads)}
}

// selfAlertRun is the alerter's component run, nil when it is off.
func selfAlertRun(deps *serverDeps, disk diskState) componentSpec {
	alerter := newSelfAlerter(deps, disk)
	if alerter == nil {
		return monitoredRun(deps, "self-alerts", nil)
	}
	return monitoredRun(deps, "self-alerts", alerter.run)
}
