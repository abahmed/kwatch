package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
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
	p.Cause = &rootcause.CauseRecord{
		Summary: "deployment app explains 4 failures", Score: 0.9}
	p.Members = map[detection.Key]detection.Finding{}
	for _, r := range []string{"a", "b", "c"} {
		p.Members[detection.Key{Entity: p.Root, Reason: r}] =
			detection.Finding{Entity: p.Root}
	}
	d := toDetail(p)
	d.RootCause = p.Cause.Summary

	assert.Equal(t, "The deployment api is the common cause of 3 "+
		"failing objects.", whyText(&d, p).Text)

	p.Members = nil
	assert.Equal(t, "The deployment api is the cause.",
		whyText(&d, p).Text)
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
