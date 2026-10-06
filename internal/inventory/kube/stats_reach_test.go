package kube

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
)

// reachPoller polls node "a" at the times the test sets and keeps the
// failure counts it submitted for the node.
type reachPoller struct {
	poller *StatsPoller
	now    time.Time
	counts []float64
	spans  []float64
	fails  map[string]error
}

func newReachPoller() *reachPoller {
	r := &reachPoller{now: fixedNow(), fails: map[string]error{}}
	r.poller = NewStatsPoller(StatsConfig{
		Kubelet: mapKubelet{failures: r.fails},
		Now:     func() time.Time { return r.now },
		Nodes:   statsNodes("a"),
		Submit: func(_ context.Context, obs ...inventory.Observation) {
			for _, o := range obs {
				if v, ok := o.Attributes[AttrKubeletFailures]; ok {
					n, _ := v.AsNumber()
					r.counts = append(r.counts, n)
					span, _ := o.Attributes[AttrKubeletFailureSpan].
						AsNumber()
					r.spans = append(r.spans, span)
				}
			}
		},
	})
	return r
}

func (r *reachPoller) pollAfter(d time.Duration, failing bool) {
	r.now = r.now.Add(d)
	if failing {
		r.fails["a"] = errors.New("dial tcp: i/o timeout")
	} else {
		delete(r.fails, "a")
	}
	r.poller.poll(context.Background())
}

func TestStatsPollerCountsRepeatedKubeletFailures(t *testing.T) {
	r := newReachPoller()

	r.pollAfter(0, false)
	r.pollAfter(10*time.Minute, true)
	r.pollAfter(10*time.Minute, true)
	r.pollAfter(10*time.Minute, true)

	assert.Equal(t, []float64{1, 2, 3}, r.counts,
		"a healthy node costs no observation")
}

func TestStatsPollerClearsTheCountWhenTheKubeletAnswers(t *testing.T) {
	r := newReachPoller()
	r.pollAfter(0, true)
	r.pollAfter(time.Minute, true)

	r.pollAfter(time.Minute, false)
	r.pollAfter(time.Minute, false)

	assert.Equal(t, []float64{1, 2, 0}, r.counts,
		"the count falls to zero once, then submissions stop")
}

func TestStatsPollerForgetsFailuresOutsideTheWindow(t *testing.T) {
	r := newReachPoller()
	r.pollAfter(0, true)
	r.pollAfter(KubeletFailureWindow+time.Minute, true)

	assert.Equal(t, []float64{1, 1}, r.counts)
}

func TestStatsPollerReportsHowLongTheFailuresSpan(t *testing.T) {
	r := newReachPoller()
	r.pollAfter(0, true)
	r.pollAfter(20*time.Minute, true)

	assert.Equal(t, []float64{0, 1200}, r.spans)
}

func TestStatsPollerClearsTheCountWhenTheKubeletStartsRefusing(
	t *testing.T,
) {
	r := newReachPoller()
	r.pollAfter(0, true)
	r.pollAfter(time.Minute, true)
	r.fails["a"] = codedError{code: http.StatusUnauthorized}
	r.poller.poll(context.Background())

	assert.Equal(t, []float64{1, 2, 0}, r.counts)
}

func TestStatsPollerDoesNotCountPermissionRefusals(t *testing.T) {
	r := newReachPoller()
	r.fails["a"] = codedError{code: http.StatusForbidden}
	r.poller.poll(context.Background())

	assert.Empty(t, r.counts)
}
