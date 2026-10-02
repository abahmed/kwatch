package kube

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Stats reachability reasons are the complete bounded vocabulary of
// StatsRound.Reason. Health publishes them as-is.
const (
	// StatsReasonUnreachable means no polled kubelet answered.
	StatsReasonUnreachable = "kubelet_unreachable"
	// StatsReasonPartial means some kubelets did not answer, or the round
	// ran out of time before every node was polled.
	StatsReasonPartial = "kubelet_partially_unreachable"
	// StatsReasonPermissionDenied means a kubelet refused the request
	// (HTTP 401 or 403), usually missing nodes/stats RBAC.
	StatsReasonPermissionDenied = "optional_permission_denied"
	// StatsReasonNotConfigured means the poller has no kubelet client.
	StatsReasonNotConfigured = "source_not_configured"
)

// statsLogEvery spaces out default-level failure logs, so a cluster with
// unreachable kubelets does not log every poll interval.
const statsLogEvery = 10 * time.Minute

// StatsRound summarises one poll round for health and metrics. It holds
// counts and a bounded reason only, never node names or error text.
type StatsRound struct {
	// Nodes is how many nodes the round meant to poll.
	Nodes int
	// Failed counts nodes whose summary could not be read or decoded.
	Failed int
	// Skipped counts nodes the round ran out of time to start.
	Skipped int
	// Reason is empty when every node answered.
	Reason string
}

// statusCoder is implemented by kubelet client errors that carry the HTTP
// status, so a refusal can be told apart from an unreachable node.
type statusCoder interface {
	StatusCode() int
}

// statsTally collects node results from the round's goroutines.
type statsTally struct {
	mu         sync.Mutex
	failed     int
	denied     bool
	firstNode  string
	firstError error
}

func (t *statsTally) add(node inventory.EntityID, err error) {
	if err == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failed++
	var coded statusCoder
	if errors.As(err, &coded) {
		code := coded.StatusCode()
		t.denied = t.denied || code == http.StatusUnauthorized ||
			code == http.StatusForbidden
	}
	if t.firstError == nil {
		t.firstNode, t.firstError = node.Name, err
	}
}

// round builds the summary once every goroutine of the round finished.
func (t *statsTally) round(nodes, started int) StatsRound {
	r := StatsRound{Nodes: nodes, Failed: t.failed, Skipped: nodes - started}
	switch {
	case r.Failed == 0 && r.Skipped == 0:
	case t.denied:
		r.Reason = StatsReasonPermissionDenied
	case r.Failed == nodes:
		r.Reason = StatsReasonUnreachable
	default:
		r.Reason = StatsReasonPartial
	}
	return r
}

// rotate returns nodes starting at the node the previous round did not
// reach, so a round that runs out of time never starves the same nodes.
func (p *StatsPoller) rotate(nodes []inventory.EntityID) []inventory.EntityID {
	if len(nodes) == 0 {
		return nodes
	}
	start := p.next % len(nodes)
	out := make([]inventory.EntityID, 0, len(nodes))
	out = append(out, nodes[start:]...)
	return append(out, nodes[:start]...)
}

// advance records where the next round starts: just past the last node
// this round started.
func (p *StatsPoller) advance(nodes, started int) {
	if nodes == 0 {
		p.next = 0
		return
	}
	p.next = (p.next%nodes + started) % nodes
}

// finishRound logs failures at the default level at most once per
// statsLogEvery, logs recovery once, and hands the round to Report.
func (p *StatsPoller) finishRound(
	now time.Time, tally *statsTally, round StatsRound,
) {
	switch {
	case round.Reason == "" && p.failing:
		p.failing = false
		klog.InfoS("kubelet stats reachable again",
			"component", "kubelet-stats", "operation", "poll",
			"nodes", round.Nodes)
	case round.Reason != "":
		p.failing = true
		if p.lastLog.IsZero() || now.Sub(p.lastLog) >= statsLogEvery {
			p.lastLog = now
			klog.InfoS("kubelet stats unavailable",
				"component", "kubelet-stats", "operation", "poll",
				"reason", round.Reason, "nodes", round.Nodes,
				"failed", round.Failed, "skipped", round.Skipped,
				"exampleNode", tally.firstNode, "error", tally.firstError)
		}
	}
	if p.cfg.Report != nil {
		p.cfg.Report(round)
	}
}

// reportNoClient tells Report the poller cannot read any kubelet.
func (p *StatsPoller) reportNoClient(ctx context.Context) bool {
	if p.cfg.Kubelet != nil {
		return false
	}
	if p.cfg.Report != nil && ctx.Err() == nil {
		p.cfg.Report(StatsRound{Reason: StatsReasonNotConfigured})
	}
	return true
}
