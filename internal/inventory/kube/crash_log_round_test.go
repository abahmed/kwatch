package kube_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const redisLine = "Unhandled exception. StackExchange.Redis." +
	"RedisConnectionException: It was not possible to connect to the " +
	"redis server(s). Error connecting right now."

// previousLogs serves canned previous-run lines and counts reads.
type previousLogs struct {
	lines map[string][]string
	err   error
	reads []inventory.EntityID
}

func (p *previousLogs) PreviousLines(
	_ context.Context, id inventory.EntityID,
) ([]string, error) {
	p.reads = append(p.reads, id)
	return p.lines[id.Name], p.err
}

// crashRig is a model, a fake log reader and the observations submitted.
type crashRig struct {
	model     *inventory.Model
	logs      *previousLogs
	round     *kube.CrashLogRound
	submitted []inventory.Observation
}

func newCrashRig(t *testing.T) *crashRig {
	t.Helper()
	r := &crashRig{
		model: inventory.NewModel(inventory.Options{
			EnrichmentSources: kube.EnrichmentSources()}),
		logs: &previousLogs{lines: map[string][]string{}},
	}
	r.round = kube.NewCrashLogRound(kube.CrashLogConfig{
		Logs: r.logs, Model: r.model, Now: fixedTime,
		Submit: func(_ context.Context, o ...inventory.Observation) {
			r.submitted = append(r.submitted, o...)
		},
	})
	return r
}

// crash adds a container that crash-looped restarts times.
func (r *crashRig) crash(
	t *testing.T, pod string, restarts float64,
) inventory.EntityID {
	t.Helper()
	id := kube.ContainerID(testNamespace, pod, "app")
	r.apply(t, id, map[string]inventory.Value{
		kube.AttrRestarts:     inventory.Number(restarts),
		kube.AttrState:        inventory.Text("waiting"),
		kube.AttrStateReason:  inventory.Text("CrashLoopBackOff"),
		kube.AttrLastExitCode: inventory.Number(143),
		kube.AttrLastFinished: inventory.Time(fixedTime()),
	})
	return id
}

func (r *crashRig) apply(
	t *testing.T, id inventory.EntityID, attrs map[string]inventory.Value,
) {
	t.Helper()
	_, err := r.model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: kube.ObservationSource,
		At: fixedTime(), Entity: id, Attributes: attrs,
	})
	require.NoError(t, err)
}

func TestCrashLogRoundReadsOncePerRestart(t *testing.T) {
	r := newCrashRig(t)
	id := r.crash(t, "p1", 3)
	r.logs.lines["p1/app"] = []string{"info: start", redisLine}
	r.round.Round(context.Background())
	r.round.Round(context.Background())
	require.Len(t, r.logs.reads, 1)
	require.Len(t, r.submitted, 1)
	assert.Equal(t, id, r.submitted[0].Entity)
	assert.Equal(t, redisLine, r.submitted[0].
		Attributes[kube.AttrLastErrorLine].AsText())

	r.crash(t, "p1", 4)
	r.round.Round(context.Background())
	assert.Len(t, r.logs.reads, 2, "a new restart is read again")
}

func TestCrashLogRoundSkipsHealthyAndFirstRun(t *testing.T) {
	r := newCrashRig(t)
	healthy := kube.ContainerID(testNamespace, "ok", "app")
	r.apply(t, healthy, map[string]inventory.Value{
		kube.AttrRestarts:     inventory.Number(2),
		kube.AttrLastExitCode: inventory.Number(0)})
	r.crash(t, "fresh", 0)
	r.round.Round(context.Background())
	assert.Empty(t, r.logs.reads)
}

func TestCrashLogRoundCapsReads(t *testing.T) {
	r := newCrashRig(t)
	for i := range 30 {
		r.crash(t, fmt.Sprintf("p%02d", i), 1)
	}
	r.round.Round(context.Background())
	assert.Len(t, r.logs.reads, 20)
	r.round.Round(context.Background())
	assert.Len(t, r.logs.reads, 30, "the rest is read next round")
}

func TestCrashLogRoundQuotesFirstErrorNotStackFrame(t *testing.T) {
	r := newCrashRig(t)
	r.crash(t, "p1", 2)
	r.logs.lines["p1/app"] = []string{"info: start",
		"at Foo.Bar() in Program.cs:12", "fatal: cannot open db", "error: x"}
	r.round.Round(context.Background())
	require.Len(t, r.submitted, 1)
	assert.Equal(t, "fatal: cannot open db", r.submitted[0].
		Attributes[kube.AttrLastErrorLine].AsText())
}

func TestCrashLogRoundNoErrorLineClearsAttribute(t *testing.T) {
	r := newCrashRig(t)
	r.crash(t, "p1", 2)
	r.logs.lines["p1/app"] = []string{"info: bye"}
	r.round.Round(context.Background())
	require.Len(t, r.submitted, 1)
	assert.Empty(t, r.submitted[0].Attributes)
}

func TestCrashLogRoundForgetsGoneContainers(t *testing.T) {
	r := newCrashRig(t)
	id := r.crash(t, "p1", 1)
	r.round.Round(context.Background())
	_, err := r.model.Apply(inventory.Observation{
		Kind: inventory.Gone, Source: kube.ObservationSource,
		At: fixedTime(), Entity: id})
	require.NoError(t, err)
	r.round.Round(context.Background())
	r.crash(t, "p1", 1)
	r.round.Round(context.Background())
	assert.Len(t, r.logs.reads, 2, "state of a gone pod is dropped")
}

func TestCrashLogRoundStopsWhenForbidden(t *testing.T) {
	r := newCrashRig(t)
	r.logs.err = apierrors.NewForbidden(
		schema.GroupResource{Resource: "pods/log"}, "p1", nil)
	for i := range 5 {
		r.crash(t, fmt.Sprintf("p%d", i), 1)
	}
	r.round.Round(context.Background())
	assert.Len(t, r.logs.reads, 1)
	assert.Empty(t, r.submitted)
}

func TestCrashLogRoundRetriesAfterOtherErrors(t *testing.T) {
	r := newCrashRig(t)
	r.logs.err = fmt.Errorf("timeout")
	r.crash(t, "p1", 1)
	r.round.Round(context.Background())
	r.logs.err = nil
	r.round.Round(context.Background())
	assert.Len(t, r.logs.reads, 1, "backs off after a transient error")
	for range 3 {
		r.round.Round(context.Background())
	}
	assert.Len(t, r.logs.reads, 2, "retries once the backoff is over")
}

func TestCrashLogRoundMarksMissingPreviousLogsAsRead(t *testing.T) {
	for _, err := range []error{
		apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "p1"),
		apierrors.NewBadRequest("previous terminated container not found"),
	} {
		r := newCrashRig(t)
		r.logs.err = err
		r.crash(t, "p1", 1)
		r.round.Round(context.Background())
		r.round.Round(context.Background())
		assert.Len(t, r.logs.reads, 1, "never asked again for this run")
	}
}

func TestCrashLogRoundReachesEveryContainerDespiteFailures(t *testing.T) {
	r := newCrashRig(t)
	r.logs.err = fmt.Errorf("timeout")
	for i := range 45 {
		r.crash(t, fmt.Sprintf("p%02d", i), 1)
	}
	for range 3 {
		r.round.Round(context.Background())
	}
	seen := map[inventory.EntityID]bool{}
	for _, id := range r.logs.reads {
		seen[id] = true
	}
	assert.Len(t, seen, 45, "failing containers do not starve the rest")
}

func TestFirstErrorLineGroupsAsOneSignature(t *testing.T) {
	a := format.Signature(redisLine)
	b := format.Signature(redisLine)
	assert.Equal(t, a, b)
	assert.False(t, format.IsGenericSignature(a))
	assert.NotContains(t, a, "<ip>")
}

func TestCrashLogObservationCarriesTheErrorLine(t *testing.T) {
	o := kube.CrashLogObservation(
		kube.ContainerID(testNamespace, "p", "app"), time.Time{},
		[]string{"fatal: boom"})
	assert.Equal(t, "fatal: boom", o.Attributes[kube.AttrLastErrorLine].
		AsText())
}
