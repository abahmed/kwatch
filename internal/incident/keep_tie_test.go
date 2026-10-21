package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

const changedRow = "config-changed"

// tieRig has two announced incidents, api and web, each explained by its
// own ConfigMap (cfg-a, cfg-b). api's cause is the one that gets
// re-ranked in the tests below.
type tieRig struct {
	*rig
	api  detection.Finding
	a, b inventory.EntityID
}

func announcedTieRig(t *testing.T) *tieRig {
	t.Helper()
	r := newRig(t, Config{})
	apiPod, _ := workload(r, "api")
	webPod, _ := workload(r, "web")
	tr := &tieRig{rig: r, a: cm("cfg-a"), b: cm("cfg-b"),
		api: sig(apiPod, reasons.CrashLoopBackOff, detection.Warning)}
	web := sig(webPod, reasons.CrashLoopBackOff, detection.Warning)
	r.rule.causes[apiPod] = tieCause(tr.a, 0.50, apiPod)
	r.rule.causes[webPod] = tieCause(tr.b, 0.50, webPod)
	r.raise(at(0), tr.api, web)
	ds := r.tick(at(DefaultSettle))
	require.Len(t, ds, 2, "both incidents are announced")
	return tr
}

func cm(name string) inventory.EntityID {
	return entity(kube.KindConfigMap, name)
}

func tieCause(
	root inventory.EntityID, raw float64, covers ...inventory.EntityID,
) explain.Cause {
	return explain.Cause{Root: root, Row: changedRow, Raw: raw,
		Confidence: raw, Summary: "configmap changed", Covers: covers}
}

// rerank makes the solver explain api by to, with the alternatives it
// weighed, and tells the manager the finding changed.
func (tr *tieRig) rerank(
	now time.Time, to explain.Cause, others ...explain.Cause,
) {
	pod := tr.api.Entity
	tr.rule.causes[pod] = to
	tr.rule.alternatives = map[inventory.EntityID][]explain.Cause{
		pod: others}
	tr.apply(now, detection.Changed, tr.api)
}

// An incident announced long ago keeps its root when the solver finds a
// cause that only ties with it: the old root is a rival within the
// margin, so the re-rank is not news. Without this the finding moved to
// the other incident, and the old one was closed as "moved".
func TestAnnouncedIncidentKeepsItsRootWhenANewCauseTiesWithIt(t *testing.T) {
	tr := announcedTieRig(t)
	apiPod := tr.api.Entity
	day5 := 5 * 24 * time.Hour

	tr.rerank(at(day5), tieCause(tr.b, 0.52, apiPod),
		tieCause(tr.a, 0.50, apiPod))

	wantNone(t, tr.tick(at(day5)))
	old := tr.of(tr.a)
	assert.Equal(t, Open, old.State)
	assert.Empty(t, old.SupersededBy)
	assert.Len(t, old.Members, 1, "api still belongs to its incident")
}

// A peer that out-ranks the incumbent every few hours must not re-root
// an old incident over and over, each flip a "moved" and a new message.
func TestPeerFlipEveryFewHoursNeverMovesAnOldIncident(t *testing.T) {
	tr := announcedTieRig(t)
	apiPod := tr.api.Entity
	now := 5 * 24 * time.Hour

	for flip := range 8 {
		to, from := tr.b, tr.a
		if flip%2 == 1 {
			to, from = tr.a, tr.b
		}
		tr.rerank(at(now), tieCause(to, 0.52, apiPod),
			tieCause(from, 0.50, apiPod))
		wantNone(t, tr.tick(at(now)))
		wantNone(t, tr.tick(at(now+DefaultReviseSettle+time.Second)))
		now += 3 * time.Hour
	}
	assert.Equal(t, Open, tr.of(tr.a).State)
	assert.Empty(t, tr.of(tr.a).SupersededBy)
	assert.Equal(t, 1, tr.of(tr.a).Revision, "nothing was re-announced")
}

// The same tie in the first minutes is still a real revision: the
// picture is still forming and the first message may name the better
// cause.
func TestTieInTheFirstMinutesStillRevisesTheCause(t *testing.T) {
	tr := announcedTieRig(t)
	apiPod := tr.api.Entity
	soon := DefaultSettle + explain.TemporalExclusion/2

	tr.rerank(at(soon), tieCause(tr.b, 0.52, apiPod),
		tieCause(tr.a, 0.50, apiPod))
	tr.tick(at(soon))

	assert.Equal(t, tr.idOf(tr.b), tr.of(tr.a).SupersededBy,
		"the finding moved to the incident of the new cause")
}

// An old root that out-scores the new cause of the same row is still
// the better answer: the finding stays, though the solver's cover
// preferred the other one and no rival is recorded.
func TestOldRootThatOutscoresTheNewCauseOfTheSameRowStays(t *testing.T) {
	tr := announcedTieRig(t)
	apiPod := tr.api.Entity
	day5 := 5 * 24 * time.Hour

	tr.rerank(at(day5), tieCause(tr.b, 0.40, apiPod),
		tieCause(tr.a, 0.60, apiPod))

	wantNone(t, tr.tick(at(day5)))
	assert.Empty(t, tr.of(tr.a).SupersededBy)
	assert.Len(t, tr.of(tr.a).Members, 1)
}

// An old root that scores far below the new cause is no tie: the
// finding moves to the cause that explains it better.
func TestOldRootThatScoresFarBelowTheNewCauseDoesNotKeepTheFinding(
	t *testing.T,
) {
	tr := announcedTieRig(t)
	apiPod := tr.api.Entity
	day5 := 5 * 24 * time.Hour
	other := tieCause(tr.a, 0.20, apiPod)
	other.Row = "node-not-ready"

	tr.rerank(at(day5), tieCause(tr.b, 0.40, apiPod), other)
	tr.tick(at(day5))

	assert.Equal(t, tr.idOf(tr.b), tr.of(tr.a).SupersededBy)
}
