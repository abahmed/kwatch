package kube_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

type sampleReader struct {
	mu    sync.Mutex
	err   error
	lines []string
	reads []inventory.EntityID
}

func (r *sampleReader) SampleLines(
	_ context.Context, c inventory.EntityID, _ time.Duration,
) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads = append(r.reads, c)
	return r.lines, r.err
}

type sampleHarness struct {
	t      *testing.T
	model  *inventory.Model
	reader *sampleReader
	now    time.Time
	sent   []inventory.Observation
	round  *kube.LogSampleRound
}

func newSampleHarness(t *testing.T) *sampleHarness {
	h := &sampleHarness{t: t, model: inventory.NewModel(inventory.Options{}),
		reader: &sampleReader{}, now: fixedTime()}
	h.round = kube.NewLogSampleRound(kube.LogSampleConfig{
		Logs: h.reader, Model: h.model, Now: func() time.Time { return h.now },
		Submit: func(_ context.Context, o ...inventory.Observation) {
			h.sent = append(h.sent, o...)
		}})
	return h
}

func (h *sampleHarness) put(
	id inventory.EntityID, attrs map[string]inventory.Value,
	owner *inventory.EntityID,
) {
	_, err := h.model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: h.now, Entity: id,
		Attributes: attrs})
	require.NoError(h.t, err)
	if owner != nil {
		_, err = h.model.Apply(inventory.Observation{
			Kind: inventory.Related, Source: "test", At: h.now, Entity: id,
			Relation: inventory.OwnedBy, Targets: []inventory.EntityID{*owner}})
		require.NoError(h.t, err)
	}
}

// workload adds a Deployment with one pod and its container.
func (h *sampleHarness) workload(name string, ready bool) {
	dep := inventory.CoreID(kube.KindDeployment, "shop", name)
	pod := inventory.CoreID(kube.KindPod, "shop", name+"-0")
	h.put(dep, nil, nil)
	h.put(pod, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Running"),
		kube.AttrReady: inventory.Bool(ready)}, &dep)
	h.put(inventory.CoreID(kube.KindContainer, "shop", name+"-0/app"), nil, nil)
}

func errorLines(n int) []string {
	out := []string{"info ok"}
	for i := range n {
		out = append(out, fmt.Sprintf("error: pq: connection refused id=%d", i))
	}
	return out
}

func TestLogSampleRoundRecordsRunningReadyWorkloads(t *testing.T) {
	h := newSampleHarness(t)
	h.workload("orders", true)
	h.workload("starting", false)
	h.reader.lines = errorLines(30)

	h.round.Round(context.Background())

	require.Len(t, h.sent, 1)
	assert.Len(t, h.reader.reads, 1, "only the ready pod is read")
	assert.Equal(t, "shop", h.reader.reads[0].Namespace)
	assert.Equal(t, "orders-0/app", h.reader.reads[0].Name)
	errs, ok := h.sent[0].Attributes[kube.AttrLogErrors].AsNumber()
	assert.True(t, ok)
	assert.EqualValues(t, 30, errs)
	assert.Equal(t, "error: pq: connection refused id=0",
		h.sent[0].Attributes[kube.AttrLogTopLine].AsText())
}

func TestLogSampleRoundSamplesOncePerWindow(t *testing.T) {
	h := newSampleHarness(t)
	h.workload("orders", true)
	h.round.Round(context.Background())
	_, err := h.model.Apply(h.sent[0])
	require.NoError(t, err)
	h.now = h.now.Add(30 * time.Second)
	h.round.Round(context.Background())
	assert.Len(t, h.reader.reads, 1, "still inside the window")
	h.now = h.now.Add(2 * time.Minute)
	h.round.Round(context.Background())
	assert.Len(t, h.reader.reads, 2)
}

func TestLogSampleRoundComparesWithEarlierWindows(t *testing.T) {
	h := newSampleHarness(t)
	h.workload("orders", true)
	h.reader.lines = errorLines(2)
	for range 3 {
		h.round.Round(context.Background())
		h.now = h.now.Add(2 * time.Minute)
	}
	h.reader.lines = errorLines(50)
	h.round.Round(context.Background())

	last := h.sent[len(h.sent)-1]
	assert.EqualValues(t, 50, mustNumber(t, last, kube.AttrLogErrors))
	assert.EqualValues(t, 6, mustNumber(t, last, kube.AttrLogBaseline))
	assert.EqualValues(t, 360, mustNumber(t, last, kube.AttrLogBaselineSpan))
}

func mustNumber(t *testing.T, o inventory.Observation, name string) float64 {
	n, ok := o.Attributes[name].AsNumber()
	require.True(t, ok, name)
	return n
}

func TestLogSampleRoundPausesQuietlyWhenLogsAreForbidden(t *testing.T) {
	h := newSampleHarness(t)
	h.workload("orders", true)
	h.reader.err = apierrors.NewForbidden(
		schema.GroupResource{Resource: "pods/log"}, "orders-0",
		errors.New("no"))

	h.round.Round(context.Background())
	assert.Empty(t, h.sent, "a refusal records nothing")
	for range 5 {
		h.now = h.now.Add(30 * time.Second)
		h.round.Round(context.Background())
	}
	assert.Len(t, h.reader.reads, 1, "no further read while paused")
}

func TestLogSampleRoundBoundsReadsPerRound(t *testing.T) {
	h := newSampleHarness(t)
	for i := range 20 {
		h.workload(fmt.Sprintf("w%02d", i), true)
	}
	h.round.Round(context.Background())
	assert.LessOrEqual(t, len(h.reader.reads), 6)
	h.round.Round(context.Background())
	assert.Greater(t, len(h.reader.reads), 6, "the next round goes on")
}

func TestLogSampleRoundRunStopsWhenCancelled(t *testing.T) {
	h := newSampleHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { h.round.Run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestLogSampleObservationOfNoWindowsIsEmpty(t *testing.T) {
	id := inventory.CoreID(kube.KindDeployment, "shop", "orders")
	o := kube.LogSampleObservation(id, time.Unix(1, 0), nil)
	assert.Empty(t, o.Attributes)
}

func TestLogSampleKeepsACappedWindowsCountUnknown(t *testing.T) {
	id := inventory.CoreID(kube.KindDeployment, "shop", "orders")
	var busy []string
	for i := range 30 {
		busy = append(busy, fmt.Sprintf("error: kind%c failed", 'a'+i))
	}
	now := []string{"error: kindz failed", "error: kindz failed"}
	o := kube.LogSampleObservation(id, time.Unix(1, 0),
		[][]string{busy, now})
	_, known := o.Attributes[kube.AttrLogTopBaseline]
	assert.False(t, known, "its earlier count is unknown, not zero")
	quiet := kube.LogSampleObservation(id, time.Unix(1, 0),
		[][]string{{"error: x failed"}, now})
	_, known = quiet.Attributes[kube.AttrLogTopBaseline]
	assert.True(t, known)
}
