package kube_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const (
	answered500 = "Readiness probe failed: HTTP probe failed with " +
		"statuscode: 500"
	refusedProbe = "Readiness probe failed: Get \"http://10.0.0.9:8080/" +
		"ready\": dial tcp 10.0.0.9:8080: connect: connection refused"
	healthLine = "health check failed: database not ready"
)

// currentLogs serves canned current-run lines and counts reads.
type currentLogs struct {
	lines map[string][]string
	err   error
	reads []inventory.EntityID
}

func (c *currentLogs) CurrentRunLines(
	_ context.Context, id inventory.EntityID,
) ([]string, error) {
	c.reads = append(c.reads, id)
	return c.lines[id.Name], c.err
}

// probeRig is a crashRig whose round also reads current logs, on a
// clock the test moves.
type probeRig struct {
	*crashRig
	current *currentLogs
	now     time.Time
}

func newProbeRig(t *testing.T) *probeRig {
	t.Helper()
	r := &probeRig{crashRig: newCrashRig(t), now: fixedTime(),
		current: &currentLogs{lines: map[string][]string{}}}
	r.round = kube.NewCrashLogRound(kube.CrashLogConfig{
		Logs: r.logs, Current: r.current, Model: r.model,
		Now: func() time.Time { return r.now },
		Submit: func(_ context.Context, o ...inventory.Observation) {
			r.submitted = append(r.submitted, o...)
		},
	})
	return r
}

// failing adds a running container whose readiness probe failed with
// message.
func (r *probeRig) failing(
	t *testing.T, pod, state, message string,
) inventory.EntityID {
	t.Helper()
	id := kube.ContainerID(testNamespace, pod, "app")
	r.apply(t, id, map[string]inventory.Value{
		kube.AttrState: inventory.Text(state)})
	_, err := r.model.Apply(inventory.Observation{
		Kind: inventory.Noted, Source: kube.ObservationSource,
		At: fixedTime(), Entity: id,
		Note: inventory.Note{At: fixedTime(), Reason: "Unhealthy",
			Message: message, Count: 3, Warning: true},
	})
	require.NoError(t, err)
	return id
}

func TestProbeLogRoundQuotesTheLastFailureLine(t *testing.T) {
	r := newProbeRig(t)
	id := r.failing(t, "p1", "running", answered500)
	r.current.lines["p1/app"] = []string{"info: starting",
		"error: cache warm-up slow", healthLine, "info: request served"}

	r.round.Round(context.Background())

	require.Equal(t, []inventory.EntityID{id}, r.current.reads)
	require.Len(t, r.submitted, 1)
	assert.Equal(t, healthLine, r.submitted[0].
		Attributes[kube.AttrProbeLogLine].AsText())
	assert.Empty(t, r.logs.reads, "a running container has no crash")
}

func TestProbeLogRoundSkipsUnansweredAndStoppedProbes(t *testing.T) {
	r := newProbeRig(t)
	r.failing(t, "refused", "running", refusedProbe)
	r.failing(t, "waiting", "waiting", answered500)
	healthy := kube.ContainerID(testNamespace, "healthy", "app")
	r.apply(t, healthy, map[string]inventory.Value{
		kube.AttrState: inventory.Text("running")})

	r.round.Round(context.Background())

	assert.Empty(t, r.current.reads)
	assert.Empty(t, r.submitted)
}

func TestProbeLogRoundRereadsOnlyAfterTheInterval(t *testing.T) {
	r := newProbeRig(t)
	r.failing(t, "p1", "running", answered500)
	ctx := context.Background()
	for range 10 {
		r.round.Round(ctx)
	}
	assert.Len(t, r.current.reads, 1)
	r.round.Round(ctx)
	assert.Len(t, r.current.reads, 2, "read again five minutes later")
}

func TestProbeLogRoundClearsTheLineOnceTheProbeIsOld(t *testing.T) {
	r := newProbeRig(t)
	id := r.failing(t, "p1", "running", answered500)
	r.current.lines["p1/app"] = []string{healthLine}
	r.round.Round(context.Background())
	require.Len(t, r.submitted, 1)

	r.now = r.now.Add(6 * time.Minute)
	r.round.Round(context.Background())

	require.Len(t, r.submitted, 2)
	assert.Equal(t, id, r.submitted[1].Entity)
	assert.Empty(t, r.submitted[1].Attributes, "the line is removed")
	r.round.Round(context.Background())
	assert.Len(t, r.submitted, 2, "removed once")
}

func TestProbeLogRoundBacksOffWhenForbidden(t *testing.T) {
	r := newProbeRig(t)
	r.current.err = apierrors.NewForbidden(
		schema.GroupResource{Resource: "pods/log"}, "p1", nil)
	r.failing(t, "p1", "running", answered500)
	ctx := context.Background()

	r.round.Round(ctx)
	assert.True(t, r.round.PodLogsForbidden())
	for range 9 {
		r.round.Round(ctx)
	}
	assert.Len(t, r.current.reads, 1, "no read during the backoff")
	assert.Empty(t, r.submitted, "a refusal is never an observation")
}

func TestProbeLogRoundSharesTheReadBudgetWithCrashes(t *testing.T) {
	r := newProbeRig(t)
	for i := range 15 {
		r.crash(t, fmt.Sprintf("c%02d", i), 1)
		r.failing(t, fmt.Sprintf("p%02d", i), "running", answered500)
	}
	r.round.Round(context.Background())
	assert.Len(t, r.logs.reads, 15)
	assert.Len(t, r.current.reads, 5, "20 reads per round in all")
}

func TestProbeLogRoundWaitsAfterAFailedRead(t *testing.T) {
	r := newProbeRig(t)
	r.current.err = fmt.Errorf("stream reset")
	r.failing(t, "p1", "running", answered500)
	ctx := context.Background()
	for range 3 {
		r.round.Round(ctx)
	}
	assert.Len(t, r.current.reads, 1)
	r.round.Round(ctx)
	assert.Len(t, r.current.reads, 2, "retried after the backoff")
}

func TestProbeLogRoundWithoutCurrentReaderReadsNothing(t *testing.T) {
	r := newCrashRig(t)
	id := kube.ContainerID(testNamespace, "p1", "app")
	r.apply(t, id, map[string]inventory.Value{
		kube.AttrState: inventory.Text("running")})
	r.round.Round(context.Background())
	assert.Empty(t, r.submitted)
}

func TestLogReaderCurrentRunLinesReturnsTheReadError(t *testing.T) {
	client := fake.NewSimpleClientset(pod("p"))
	var previous []bool
	client.PrependReactor("get", "pods",
		func(a ktesting.Action) (bool, runtime.Object, error) {
			g, ok := a.(ktesting.GenericAction)
			if !ok || a.GetSubresource() != "log" {
				return false, nil, nil
			}
			opts, _ := g.GetValue().(*corev1.PodLogOptions)
			previous = append(previous, opts.Previous)
			return true, nil, apierrors.NewForbidden(
				schema.GroupResource{Resource: "pods/log"}, "p", nil)
		})
	id := kube.ContainerID(testNamespace, "p", "app")

	lines, err := kube.LogReader{Client: client}.CurrentRunLines(
		context.Background(), id)

	assert.True(t, apierrors.IsForbidden(err))
	assert.Empty(t, lines)
	assert.Equal(t, []bool{false}, previous)
}
