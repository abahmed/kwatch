package kube_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const npgsqlLine = "Npgsql.NpgsqlException (0x80004005): Failed to " +
	"connect to 10.0.2.195:5432"

// A container that stays up but is never ready writes why in its own
// log; without a restart only this read finds it, whatever the probe
// got back.
func TestProbeLogRoundReadsARunningContainerThatIsNotReady(t *testing.T) {
	r := newProbeRig(t)
	id := r.failing(t, "p1", "running", refusedProbe)
	r.apply(t, id, map[string]inventory.Value{
		kube.AttrState: inventory.Text("running"),
		kube.AttrReady: inventory.Bool(false)})
	r.current.lines["p1/app"] = []string{"info: starting", npgsqlLine}

	r.round.Round(context.Background())

	require.Equal(t, []inventory.EntityID{id}, r.current.reads)
	require.Len(t, r.submitted, 1)
	assert.Equal(t, npgsqlLine, r.submitted[0].
		Attributes[kube.AttrProbeLogLine].AsText())
}

// The read is optional work: it waits while the API server is under
// pressure.
func TestProbeLogRoundWaitsUnderAPIPressure(t *testing.T) {
	r := newProbeRig(t)
	id := r.failing(t, "p1", "running", answered500)
	r.apply(t, id, map[string]inventory.Value{
		kube.AttrState: inventory.Text("running"),
		kube.AttrReady: inventory.Bool(false)})
	r.round = kube.NewCrashLogRound(kube.CrashLogConfig{
		Logs: r.logs, Current: r.current, Model: r.model,
		Now:    func() time.Time { return r.now },
		Submit: func(context.Context, ...inventory.Observation) {},
		Paused: func() bool { return true },
	})

	r.round.Round(context.Background())

	assert.Empty(t, r.current.reads)
}
