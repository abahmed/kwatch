package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/release"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func TestCauseSentenceNamesTheReleaseInPlainWords(t *testing.T) {
	p := summaryIncident()
	p.Cause = &rootcause.CauseRecord{
		Summary: "deployment app explains 4 failures", Score: 0.9}
	d := toDetail(p)
	at := p.Onset().Add(-time.Minute)
	d.Release = &ReleaseCause{Record: release.Record{At: at,
		Images: []release.Image{{
			Before: "registry.example.com/team/app:2.1-beta.4",
			After:  "registry.example.com/team/app:2.1-beta.5"}}}}

	got := whyText(&d, p).Text

	assert.Equal(t, "It started 1 minute after the release "+
		"(image 2.1-beta.4 → 2.1-beta.5).", got)
	assert.NotContains(t, got, "explains")
}

func TestCauseSentenceFallsBackToTheCommonCause(t *testing.T) {
	p := summaryIncident()
	db := inventory.EntityID{Group: "", Kind: "service",
		Namespace: "shop", Name: "db"}
	p.Cause = &rootcause.CauseRecord{Root: db,
		Summary: "service db explains 3 failures", Score: 0.9}
	p.Members = map[detection.Key]detection.Finding{}
	for _, r := range []string{"a", "b", "c"} {
		p.Members[detection.Key{Entity: p.Root, Reason: r}] =
			detection.Finding{Entity: p.Root}
	}
	d := toDetail(p)
	d.RootCause = p.Cause.Summary

	assert.Equal(t, "The service db is the common cause of 3 "+
		"failing objects.", whyText(&d, p).Text)

	p.Members = nil
	assert.Equal(t, "The service db is the cause.",
		whyText(&d, p).Text)
}

// ownCauseIncident is api failing through its own pods, the cause
// record blaming api itself.
func ownCauseIncident() (incident.Incident, IncidentDetail) {
	p := summaryIncident()
	p.Cause = &rootcause.CauseRecord{Root: p.Root,
		Summary: "deployment api explains 3 failures", Score: 0.9}
	pod := inventory.EntityID{Kind: "pod", Namespace: "shop",
		Name: "api-7d9f-x1"}
	p.Members = map[detection.Key]detection.Finding{
		{Entity: p.Root, Reason: "a"}: {Entity: p.Root},
		{Entity: pod, Reason: "b"}:    {Entity: pod},
	}
	d := toDetail(p)
	d.RootCause = p.Cause.Summary
	return p, d
}

func TestCauseSentenceOfAnOwnWorkloadQuotesItsError(t *testing.T) {
	p, d := ownCauseIncident()
	d.CapturedLogLines = []string{"starting",
		"Unhandled exception: database connection string 'Main' " +
			"is missing."}

	got := whyText(&d, p).Text

	assert.Equal(t, "kwatch hasn't found an outside cause; its own "+
		"logs say: “Unhandled exception: database connection "+
		"string 'Main' is missing”.", got)
	assert.NotContains(t, got, "common cause")
}

func TestCauseSentenceOfAnOwnWorkloadFallsBack(t *testing.T) {
	p, d := ownCauseIncident()
	code := 1
	d.Containers = []ContainerRow{{Container: "app", Restarts: 4,
		LastReason: "Error", LastExitCode: &code}}
	assert.Equal(t, "kwatch hasn't found an outside cause; its "+
		"container app last stopped: Error (exit code 1).",
		whyText(&d, p).Text)

	d.Containers = nil
	assert.Equal(t, "kwatch hasn't found an outside cause: the "+
		"failure is in the deployment api itself.", whyText(&d, p).Text)

	p.Cause.Mode = "Config.ProbePortMismatch"
	d.FullTitle = "api in shop is failing because its probe checks a " +
		"port the container does not listen on."
	assert.Equal(t, "Its probe checks a port the container does not "+
		"listen on.", whyText(&d, p).Text)
}

func TestCauseSentenceWordsASchedulingConstraint(t *testing.T) {
	p := summaryIncident()
	p.Cause = &rootcause.CauseRecord{Root: inventory.EntityID{
		Kind: rootcause.KindScheduling,
		Name: "didn't satisfy plugin(s) [NodeAffinity]. no new " +
			"claims to deallocate,"}, Score: 0.9}
	d := toDetail(p)
	d.RootCause = "scheduling explains 1 failure"

	assert.Equal(t, "The scheduler can't place the pod: no node "+
		"matches its node affinity.", whyText(&d, p).Text)

	p.Root = p.Cause.Root
	assert.NotContains(t, whatOf(&d, p), "NodeAffinity")
}

func TestCauseSentenceLeavesOutTheObjectsOwnCreation(t *testing.T) {
	p, d := ownCauseIncident()
	p.Cause.Change = &inventory.Change{Entity: p.Root, Created: true,
		At: p.Onset().Add(-time.Minute)}

	got := whyText(&d, p).Text

	assert.NotContains(t, got, "started")
	assert.Contains(t, got, noOutsideCause)

	p.Cause.Change.Created = false
	assert.Contains(t, whyText(&d, p).Text, "It started 1 minute after "+
		"a change to")
}

func TestImageWordsKeepTheRegistryOut(t *testing.T) {
	r := release.Record{Images: []release.Image{
		{Before: "r.example.com/a:1", After: "r.example.com/b:1"},
		{Before: "x:1", After: "x:2"}}}
	assert.Equal(t, " (image a:1 → b:1, and 1 more).", releaseWhat(r))
}

func TestLastSeenFollowsTheNewestTimelineEntry(t *testing.T) {
	p := summaryIncident()
	assert.Equal(t, p.Announced, lastSeen(p))
	later := p.Announced.Add(3 * time.Hour)
	p.Timeline = append(p.Timeline, incident.Event{At: later})
	assert.Equal(t, later, lastSeen(p))
}
