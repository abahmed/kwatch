package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func modeDecision(mode detection.Mode) incident.Decision {
	return incident.Decision{Action: incident.Announce,
		Incident: incident.Incident{Mode: mode,
			Root:  inventory.CoreID(kube.KindPod, "shop", "api-1"),
			Cause: &rootcause.CauseRecord{Summary: "api crashed"}}}
}

func TestProNoteNamesTheAlertType(t *testing.T) {
	cases := []struct {
		name string
		mode detection.Mode
		want []string
	}{
		{"crash", detection.ModeCrashLoop, noteCrash},
		{"liveness probe", detection.ModeProbe, noteCrash},
		{"oom", detection.ModeOOMKilled, noteOOM},
		{"image", detection.ModeImagePull, noteImage},
		{"pending", detection.ModePending, notePending},
		{"unschedulable", detection.ModeUnschedulable, notePending},
		{"generic", detection.ModeEvicted, noteGeneric},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, proLineText(tc.want),
				proNote(withChange(modeDecision(tc.mode)), helpNone))
		})
	}
}

// withChange is d with a recent change the engine found.
func withChange(d incident.Decision) incident.Decision {
	d.Facts.Changes = []inventory.Change{{}}
	return d
}

func TestProNoteTeasesAWithheldCause(t *testing.T) {
	d := modeDecision(detection.ModeCrashLoop)
	d.Incident.Cause, d.Incident.ProCause = nil, true
	assert.Equal(t, "Pro shows the likely cause it found, free during "+
		"early access: kwatch.dev/pro", proNote(d, helpNone))

	changed := withChange(modeDecision(detection.ModeCrashLoop))
	changed.Incident.Cause = nil
	assert.Equal(t, "Pro shows what changed just before this, free "+
		"during early access: kwatch.dev/pro", proNote(changed, helpNone))
}

// Without a change found, no note promises one: it names what Pro does
// show, or nothing.
func TestProNotePromisesAChangeOnlyWhenOneIsKnown(t *testing.T) {
	d := modeDecision(detection.ModeCrashLoop)
	d.Facts.Output = []string{"panic: boom"}
	assert.Equal(t, "Pro shows the error from its log and how to check "+
		"it, free during early access: kwatch.dev/pro",
		proNote(d, helpCheck))

	generic := modeDecision(detection.ModeEvicted)
	generic.Incident.Impact = []inventory.EntityID{{}}
	assert.Equal(t, proLineText([]string{noteImpact}),
		proNote(generic, helpNone))

	assert.Equal(t, proLineText([]string{proCheckWords}),
		proNote(modeDecision(detection.ModeImagePull), helpCheck))
	assert.Equal(t, proLineText(notePending),
		proNote(modeDecision(detection.ModePending), helpNone))
}

func TestProNoteSkipsTheTeaserWhenTheCauseIsNamed(t *testing.T) {
	d := modeDecision(detection.ModeCrashLoop)
	d.Incident.ProCause = true
	assert.Equal(t, proLineText(noteCrash), proNote(withChange(d), helpNone))
}

func TestProNoteIsEmptyWhenProShowsNothing(t *testing.T) {
	d := modeDecision(detection.ModeEvicted)
	d.Incident.Root = inventory.CoreID(kube.KindDeployment, "shop", "api")
	assert.Empty(t, proNote(d, helpNone))
}

func TestProNoteFollowsHideProHints(t *testing.T) {
	d := modeDecision(detection.ModeCrashLoop)
	d.Incident.Cause, d.Incident.ProCause = nil, true
	community := Writer{}.ForEdition(feature.CommunityEntitlements(), false)
	quiet := Writer{}.ForEdition(feature.CommunityEntitlements(), true)
	pro := Writer{}.ForEdition(feature.ProEntitlements(), false)

	noteOf := func(w Writer) string {
		_, note := w.communityOf(d, at(4, 0))
		return note.plain
	}
	assert.Equal(t, proLineText([]string{proTeaser}), noteOf(community))
	assert.Empty(t, noteOf(quiet))
	assert.Empty(t, noteOf(pro))
	d.Action = incident.Resolve
	assert.Empty(t, noteOf(community), "a resolve gets no Pro line")
}

func TestProNoteJoinsTheFixHintInOneLine(t *testing.T) {
	d := modeDecision(detection.ModeCrashLoop)
	assert.Equal(t, "Pro shows the change that caused this crash and how "+
		"to fix it, free during early access: kwatch.dev/pro",
		proNote(withChange(d), helpFix))
	d.Incident.Cause, d.Incident.ProCause = nil, true
	assert.Equal(t, proLineText([]string{proTeaser, proFixWords}),
		proNote(d, helpFix))

	alone := modeDecision(detection.ModeEvicted)
	alone.Incident.Root = inventory.CoreID(kube.KindDeployment, "shop",
		"api")
	assert.Equal(t, "Pro shows how to fix it"+proInvite,
		proNote(alone, helpFix))
}

func TestProNoteIsAtMostOneLine(t *testing.T) {
	d := modeDecision(detection.ModeCrashLoop)
	d.Incident.Cause, d.Incident.ProCause = nil, true
	community := Writer{}.ForEdition(feature.CommunityEntitlements(), false)
	got := community.Write(d, at(4, 0))
	assert.Equal(t, 1, strings.Count(got.Note, "kwatch.dev/pro"))
}

func TestProChecksTextCountsSinceStart(t *testing.T) {
	assert.Empty(t, ProChecksText(feature.WithheldCounts{}))
	got := ProChecksText(feature.WithheldCounts{4, 2, 1, 0})
	assert.Equal(t, "Pro checks found 7 issues since kwatch started: "+
		"4 readiness probes, 2 shared causes, 1 preemption and "+
		"admission. Pro shows them, free during early access: "+
		"kwatch.dev/pro", got)
	assert.Contains(t, ProChecksText(feature.WithheldCounts{0, 0, 0, 1}),
		"found 1 issue since kwatch started: 1 scheduling gate. Pro "+
			"shows it,")
}

func TestProChecksTodayTextCountsToday(t *testing.T) {
	assert.Empty(t, ProChecksTodayText(feature.WithheldCounts{}))
	got := ProChecksTodayText(feature.WithheldCounts{1, 0, 0, 2})
	assert.Equal(t, "Pro checks found 3 issues today: 1 readiness probe, "+
		"2 scheduling gates. Pro shows them, free during early access: "+
		"kwatch.dev/pro", got)
}
