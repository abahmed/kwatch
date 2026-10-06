package kube

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

const (
	// AttrKubeletFailures is how many times in the last
	// KubeletFailureWindow kwatch could not read a node's kubelet. It is
	// written only while the count is above zero, and once more when it
	// falls back to zero.
	AttrKubeletFailures = "kubelet.unreachable.count"
	// AttrKubeletFailureSpan is the seconds between the first and the
	// last of those failures. Failures bunched in one moment are a
	// blip; a node worth reporting fails over time.
	AttrKubeletFailureSpan = "kubelet.unreachable.span.seconds"
	// KubeletFailureWindow is the span AttrKubeletFailures counts over.
	KubeletFailureWindow = 6 * time.Hour
)

// reachLog remembers, per node, when the kubelet could not be read since
// it last answered. The per-round log line and health reason cannot say
// that one node fails again and again; this can. A successful read
// forgets the node's failures: a kubelet that answers is reachable.
type reachLog struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

func newReachLog() *reachLog {
	return &reachLog{failures: map[string][]time.Time{}}
}

// note records one poll of a node and returns the failures since the
// kubelet last answered (inside the window) and the time they span.
// changed is false while the node has no failures at all, so healthy
// nodes cost no observation.
func (l *reachLog) note(
	node string, now time.Time, failed bool,
) (count int, span time.Duration, changed bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	known := l.failures[node]
	var kept []time.Time
	if failed {
		kept = append(within(known, now.Add(-KubeletFailureWindow)), now)
	}
	if len(kept) == 0 {
		delete(l.failures, node)
		return 0, 0, len(known) > 0
	}
	l.failures[node] = kept
	return len(kept), kept[len(kept)-1].Sub(kept[0]), true
}

// within keeps the times at or after cutoff.
func within(times []time.Time, cutoff time.Time) []time.Time {
	var out []time.Time
	for _, at := range times {
		if !at.Before(cutoff) {
			out = append(out, at)
		}
	}
	return out
}

// forgetBefore drops nodes whose last failure is older than cutoff, so
// a node that left the cluster does not stay in the log.
func (l *reachLog) forgetBefore(cutoff time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for node, times := range l.failures {
		if len(within(times, cutoff)) == 0 {
			delete(l.failures, node)
		}
	}
}

// recordReach counts a failed read of a node's kubelet and submits the
// running count and its time span for the node, so a detector can see a
// node that keeps failing. A successful read, or a refusal (HTTP 401 or
// 403, a permission problem that health already reports), clears the
// node's count.
func (p *StatsPoller) recordReach(
	ctx context.Context, node inventory.EntityID, now time.Time, err error,
) {
	if ctx.Err() != nil {
		return
	}
	failed := err != nil && !refused(err)
	count, span, changed := p.reach.note(node.Name, now, failed)
	if !changed {
		return
	}
	p.cfg.Submit(ctx, inventory.Observation{
		Kind: inventory.Observed, Source: reachSource, At: now,
		Entity: node, Attributes: map[string]inventory.Value{
			AttrKubeletFailures: inventory.Number(float64(count)),
			AttrKubeletFailureSpan: inventory.Number(
				span.Seconds()),
		},
	})
}

func refused(err error) bool {
	var coded statusCoder
	if !errors.As(err, &coded) {
		return false
	}
	code := coded.StatusCode()
	return code == http.StatusUnauthorized || code == http.StatusForbidden
}
