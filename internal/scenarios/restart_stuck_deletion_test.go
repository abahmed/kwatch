package scenarios

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/pipeline"
	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// stuckDays is how many days the stuck deletion is replayed.
const stuckDays = 64

// A deletion that has been stuck for most of a year, on a cluster where
// kwatch restarts twice a day (a morning wake-up and a midday rollout)
// and stops every evening. Another stuck object of the same attacher is
// listed on some starts and not on others. Nothing about the problem
// changes: after its first message it is said again at most once in 30
// days, never "introduced", never reminded in the first minutes after a
// start, and a restart does not write its joining on the timeline
// again. The incident was opened before the old-problem horizon
// existed, so it is a notification, not a digest line.
func TestStuckDeletionStaysQuietAcrossRestarts(t *testing.T) {
	store := newPersistingStore()
	var prev *cluster
	var own []time.Time
	var first time.Time
	var last replay.Result
	for day := range stuckDays {
		for i, at := range []time.Duration{
			6*time.Hour + 45*time.Minute, 12*time.Hour + 30*time.Minute,
		} {
			start := time.Date(2026, 10, 1+day, 0, 0, 0, 0,
				time.UTC).Add(at)
			c := stuckSession(start, prev, day, i)
			log := c.log()
			log.Start = start
			end := start.Add(5*time.Hour + 15*time.Minute)
			last = stuckReplay(t, log, store, replay.Options{
				SyncAt: start, Tail: end.Sub(c.now)})
			own = append(own, ownMessages(t, last, start)...)
			if first.IsZero() && len(last.Carried) > 0 {
				first = last.Carried[0].At
			}
			prev = c
		}
	}
	require.NotEmpty(t, last.Incidents)
	require.False(t, first.IsZero(), "the first start told nothing")
	told := append([]time.Time{first}, own...)
	for i := 1; i < len(told); i++ {
		assert.GreaterOrEqual(t, told[i].Sub(told[i-1]),
			incident.UnchangedFor, "said again %s after the previous "+
				"message", told[i].Sub(told[i-1]))
	}
	assert.LessOrEqual(t, len(own), 1+stuckDays/30, "own messages: %v",
		own)
	for _, p := range last.Incidents {
		joined := 0
		for _, e := range p.Timeline {
			if strings.HasPrefix(e.Text, "Deletion is blocked") &&
				strings.Contains(e.Text, "csi-6a7b8c") {
				joined++
			}
		}
		assert.LessOrEqual(t, joined, 1, "%s wrote its join note %d "+
			"times", p.ID, joined)
	}
}

// stuckSession is one start: the first one lists the stuck attachment,
// its volume and its peer; later ones list what the previous one left,
// and the peer comes and goes.
func stuckSession(start time.Time, prev *cluster, day, i int) *cluster {
	c := newCluster(start, "")
	if prev == nil {
		c.list(c.node("n1", "zone-a"), &corev1.PersistentVolume{
			ObjectMeta: c.meta("", "pvc-3333")})
		c.list(attachmentDeleted(c, "csi-6a7b8c", "pvc-3333",
			"removed-node", 320*24*time.Hour))
		c.list(stuckPeer(c))
		return c
	}
	var kept []runtime.Object
	for _, o := range remaining(prev) {
		if !strings.Contains(objectName(o), "csi-peer") {
			kept = append(kept, o)
		}
	}
	c.list(kept...)
	if (day+i)%3 == 0 {
		c.after(time.Hour)
		c.create(stuckPeer(c))
	}
	return c
}

// stuckPeer is another attachment of the same attacher, as old.
func stuckPeer(c *cluster) runtime.Object {
	return attachmentDeleted(c, "csi-peer", "pvc-peer", "removed-node",
		310*24*time.Hour)
}

func objectName(o runtime.Object) string {
	if named, ok := o.(interface{ GetName() string }); ok {
		return named.GetName()
	}
	return ""
}

// ownMessages are the times of the incidents' own messages of one start,
// and checks that none introduces an incident or reminds of one inside
// the first minutes after the start.
func ownMessages(
	t *testing.T, run replay.Result, start time.Time,
) []time.Time {
	t.Helper()
	var out []time.Time
	for i, d := range run.Decisions {
		if d.Incident.ID == "" {
			continue
		}
		assert.NotEqual(t, incident.ReasonIntroduced, d.Reason,
			"introduced at %s", run.Times[i])
		out = append(out, run.Times[i])
	}
	for _, c := range append(run.Carried, carriedOwn(run)...) {
		if c.Decision.Reason == incident.ReasonReminder {
			assert.False(t, c.At.Before(start.Add(10*time.Minute)),
				"reminded %s after the start",
				c.At.Sub(start))
		}
	}
	return out
}

// carriedOwn are the incidents' own decisions as carried entries, so
// their reminders are checked with the carried ones.
func carriedOwn(run replay.Result) []replay.CarriedDecision {
	var out []replay.CarriedDecision
	for i, d := range run.Decisions {
		out = append(out, replay.CarriedDecision{Decision: d,
			At: run.Times[i]})
	}
	return out
}

// stuckReplay is restartReplay with no old-problem horizon, as for an
// incident opened before the horizon existed.
func stuckReplay(
	t *testing.T, log replay.Log, store *persistingStore,
	opts replay.Options,
) replay.Result {
	t.Helper()
	deps := newDependencies()
	deps.Incidents = withoutHorizon()
	deps.Store = store
	return mustReplay(t, log, deps, opts)
}

// withoutHorizon is the scenario manager with the old-problem horizon
// off.
func withoutHorizon() *incident.Manager {
	policy := config.RuntimeConfigFor(config.DefaultConfig()).Policy()
	return incident.NewManager(incident.Config{
		SeverityByReason:    policy.SeverityByReason(),
		SeverityByOwnerKind: policy.SeverityByOwnerKind(),
		IDNonce:             "5c3e",
		OwnNamespace:        "kwatch", OwnWorkload: "kwatch",
	}, ablated{explain.NewSolver(), explain.Ablation{}})
}

func mustReplay(
	t *testing.T, log replay.Log, deps pipeline.Dependencies,
	opts replay.Options,
) replay.Result {
	t.Helper()
	result, err := replay.Run(t.Context(), log, deps, opts)
	require.NoError(t, err)
	return result
}
