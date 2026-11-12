package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// summaryIncident is an open crash loop of Deployment api in shop.
func summaryIncident() incident.Incident {
	p := testIncident("s1", "shop", -time.Hour)
	p.Root.Name = "api"
	p.Mode = detection.ModeCrashLoop
	p.Headline = "api in shop keeps failing"
	return p
}

func TestSummaryWhatPrefersNotServingOverTheFamily(t *testing.T) {
	p := summaryIncident()
	d := toDetail(p)
	d.Traffic = []TrafficRow{{Kind: "Service", Name: "api", Total: 3}}

	assert.Equal(t, "Deployment api in shop is not serving traffic: "+
		"Service api has 0 of 3 endpoints ready.", whatOf(&d, p))
}

func TestSummaryWhatNamesAFailingReadinessCheck(t *testing.T) {
	p := summaryIncident()
	d := toDetail(p)
	d.Probes = []ProbeRow{{Probe: "liveness"}, {Probe: "readiness"}}

	assert.Equal(t, "Deployment api in shop is failing its readiness "+
		"check, so it gets no traffic.", whatOf(&d, p))
}

func TestSummaryWhatCountsRestartsOfACrashLoop(t *testing.T) {
	p := summaryIncident()
	d := toDetail(p)
	d.Containers = []ContainerRow{{Restarts: 2}, {Restarts: 12}}

	assert.Equal(t, "Deployment api in shop keeps crashing (12 restarts).",
		whatOf(&d, p))
}

func TestSummaryWhatSaysPendingPodsAndPastTense(t *testing.T) {
	p := summaryIncident()
	p.Mode = detection.ModeUnschedulable
	d := toDetail(p)
	d.Waiting = &Waiting{Pods: 3}
	assert.Equal(t, "Deployment api in shop is stuck pending: no node "+
		"can run it (3 pods waiting).", whatOf(&d, p))

	p.State = incident.Resolved
	assert.Equal(t, "Deployment api in shop was stuck pending.",
		whatOf(&d, p))
}

func TestSummaryWhatFallsBackToTheHeadline(t *testing.T) {
	p := summaryIncident()
	p.Mode = "LeaseStale"
	d := toDetail(p)

	assert.Equal(t, "api in shop keeps failing.", whatOf(&d, p))
}

// A what that would only repeat the title says the latest finding
// instead: the root's first, the open one while it fails, the last one
// once resolved.
func TestSummaryWhatFallsBackToTheLatestFinding(t *testing.T) {
	p := summaryIncident()
	p.Mode = "LeaseStale"
	p.Problem = "api in shop keeps failing."
	other := p.Root
	other.Name = "db"
	p.Members = map[detection.Key]detection.Finding{
		{Entity: p.Root, Reason: "old"}: {Entity: p.Root,
			Summary: "1 of 3 Service backends are ready",
			Since:   time.Now().Add(-time.Hour)},
		{Entity: p.Root, Reason: "new"}: {Entity: p.Root,
			Summary: "2 of 3 Service backends are ready",
			Since:   time.Now().Add(-time.Minute)},
		{Entity: other, Reason: "x"}: {Entity: other,
			Summary: "db is slow", Since: time.Now()},
		{Entity: p.Root, Reason: "risk"}: {Entity: p.Root,
			Summary: "no readiness probe", Advisory: true,
			Since: time.Now()},
	}
	d := toDetail(p)
	assert.Equal(t, "2 of 3 Service backends are ready.", whatOf(&d, p))

	p.State = incident.Resolved
	assert.Equal(t, "2 of 3 Service backends are ready.", whatOf(&d, p))

	p.Members = map[detection.Key]detection.Finding{
		{Entity: p.Root, Reason: "same"}: {Entity: p.Root,
			Summary: "API in shop keeps failing"},
	}
	assert.Equal(t, "api in shop keeps failing.", whatOf(&d, p),
		"a finding that only repeats the title is no better")
}

func TestSummaryWhyWordsTheConfidence(t *testing.T) {
	p := summaryIncident()
	d := toDetail(p)
	assert.Equal(t, SummaryWhy{}, whyText(&d, p))

	cases := []struct {
		score float64
		rival bool
		want  string
	}{
		{0.9, false, "high"}, {0.9, true, "likely"},
		{0.55, false, "likely"}, {0.3, false, "possible"},
	}
	for _, c := range cases {
		p.Cause = &rootcause.CauseRecord{Summary: "the image is wrong",
			Score: c.score, Root: inventory.EntityID{Kind: "configmap",
				Namespace: "shop", Name: "settings"}}
		if c.rival {
			p.Cause.Rival = &rootcause.CauseRecord{}
		}
		d := toDetail(p)
		got := whyText(&d, p)
		assert.Equal(t, SummaryWhy{Text: "The configmap settings is the " +
			"cause.",
			Confidence: c.want, CauseKnown: true}, got)
	}
}

func TestSummaryNextIsTheFirstStepWithTheRestCounted(t *testing.T) {
	assert.Nil(t, nextOf(nil))
	got := nextOf([]FixStep{{Text: "Roll back", Command: "kubectl x"},
		{Text: "b"}, {Text: "c"}})
	assert.Equal(t, &SummaryNext{Text: "Roll back", Command: "kubectl x",
		More: 2}, got)
}

func TestSummaryStageFollowsTheState(t *testing.T) {
	p := summaryIncident()
	cases := []struct {
		state incident.State
		step  string
		flap  bool
	}{
		{incident.Settling, stageConfirming, false},
		{incident.Open, stageHappening, false},
		{incident.Flapping, stageHappening, true},
		{incident.Recovering, stageRecovering, false},
		{incident.Resolved, stageResolved, false},
	}
	for _, c := range cases {
		p.State = c.state
		d := toDetail(p)
		got := stageOf(&d, p)
		assert.Equal(t, c.step, got.Step, c.state.String())
		assert.Equal(t, c.flap, got.Flapping, c.state.String())
	}
}

func TestSummaryStageSinceIsTheLatestTransitionIntoTheState(t *testing.T) {
	p := summaryIncident()
	p.State = incident.Recovering
	p.RecoveringSince = testEpoch.Add(-5 * time.Minute)
	d := toDetail(p)
	assert.True(t, p.RecoveringSince.Equal(stageOf(&d, p).Since))

	at := testEpoch.Add(-2 * time.Minute)
	d.Transitions = []Transition{
		{At: testEpoch.Add(-9 * time.Minute), To: "recovering"},
		{At: at, To: "recovering"}, {At: testEpoch, To: "open"}}
	assert.True(t, at.Equal(stageOf(&d, p).Since))

	d.StoppedTracking = true
	assert.True(t, stageOf(&d, p).StoppedTracking)
}

func TestSummaryIsServedOnTheDetail(t *testing.T) {
	p := summaryIncident()
	p.Steps = []incident.FixStep{{Text: "Read the logs",
		Command: "kubectl logs deploy/api -n shop --previous"}}
	p.Facts = []string{"starting", "panic: nil map"}
	s := newTestServer(p)

	d := detailOf(t, s, "s1")

	require.NotNil(t, d.Summary)
	assert.Equal(t, "Deployment api in shop keeps crashing.",
		d.Summary.What)
	assert.Equal(t, stageHappening, d.Summary.Stage.Step)
	require.NotNil(t, d.Summary.Next)
	assert.Equal(t, "Read the logs", d.Summary.Next.Text)
	require.Len(t, d.Summary.Proof, 1)
	assert.Equal(t, "panic: nil map", d.Summary.Proof[0].Detail)
	assert.Empty(t, d.Summary.Told)
}
