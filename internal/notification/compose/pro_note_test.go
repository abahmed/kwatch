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
		want string
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
			assert.Equal(t, tc.want+proInvite,
				proNote(modeDecision(tc.mode)))
		})
	}
}

func TestProNoteTeasesAWithheldCause(t *testing.T) {
	d := modeDecision(detection.ModeCrashLoop)
	d.Incident.Cause, d.Incident.ProCause = nil, true
	assert.Equal(t, proTeaser+proInvite, proNote(d))

	changed := modeDecision(detection.ModeCrashLoop)
	changed.Incident.Cause = nil
	changed.Facts.Changes = []inventory.Change{{}}
	assert.Equal(t, proTeaser+proInvite, proNote(changed))
}

func TestProNoteSkipsTheTeaserWhenTheCauseIsNamed(t *testing.T) {
	d := modeDecision(detection.ModeCrashLoop)
	d.Incident.ProCause = true
	assert.Equal(t, noteCrash+proInvite, proNote(d))
}

func TestProNoteIsEmptyWhenProShowsNothing(t *testing.T) {
	d := modeDecision(detection.ModeEvicted)
	d.Incident.Root = inventory.CoreID(kube.KindDeployment, "shop", "api")
	assert.Empty(t, proNote(d))
}

func TestProNoteFollowsHideProHints(t *testing.T) {
	d := modeDecision(detection.ModeCrashLoop)
	d.Incident.Cause, d.Incident.ProCause = nil, true
	community := Writer{}.ForEdition(feature.CommunityEntitlements(), false)
	quiet := Writer{}.ForEdition(feature.CommunityEntitlements(), true)
	pro := Writer{}.ForEdition(feature.ProEntitlements(), false)

	assert.Equal(t, proTeaser+proInvite, community.noteFor(d))
	assert.Empty(t, quiet.noteFor(d))
	assert.Empty(t, pro.noteFor(d))
	d.Action = incident.Resolve
	assert.Empty(t, community.noteFor(d), "a resolve gets no Pro line")
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
		"admission. Get Pro free during early access to see them: "+
		"kwatch.dev/pro", got)
	assert.Contains(t, ProChecksText(feature.WithheldCounts{0, 0, 0, 1}),
		"found 1 issue since kwatch started: 1 scheduling gate.")
}
