package incident

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func kinds(ts []Transition) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.Kind)
	}
	return out
}

func TestJournalRecordsEachStateChange(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.raise(at(0), web)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, ReasonSettled)
	r.clear(at(2*time.Minute), web)
	r.tick(at(2 * time.Minute))
	r.raise(at(3*time.Minute), web)
	r.tick(at(3 * time.Minute))
	r.clear(at(4*time.Minute), web)
	r.tick(at(4 * time.Minute))
	r.tick(at(30 * time.Minute))

	got := r.only().Journal.Transitions
	assert.Equal(t, []string{TransitionOpened, TransitionAnnounced,
		TransitionRecovering, TransitionFailingAgain,
		TransitionRecovering, TransitionResolved}, kinds(got))
	assert.Equal(t, Transition{At: at(DefaultSettle),
		Kind: TransitionAnnounced, From: "settling", To: "open"}, got[1])
	assert.Equal(t, at(0), got[0].At)
}

func TestJournalRecordsTierRiseAndReopen(t *testing.T) {
	r, start := announcedRig(t)
	r.m.mu.Lock()
	p := r.m.lookup(entity(kube.KindPod, "web"))
	p.Tier = Page
	r.m.mu.Unlock()
	r.tick(start.Add(time.Minute))
	got := r.only().Journal.Transitions
	last := got[len(got)-1]
	assert.Equal(t, Transition{At: start.Add(time.Minute),
		Kind: TransitionTierRaised, From: "notify", To: "page"}, last)

	r.m.mu.Lock()
	p.State, p.Resolved = Resolved, start.Add(2*time.Minute)
	p.Journal.seenState = Resolved
	r.m.reopen(p, start.Add(3*time.Minute))
	r.m.observe(p, start.Add(3*time.Minute))
	r.m.mu.Unlock()
	got = r.only().Journal.Transitions
	assert.Equal(t, []string{TransitionReopened, TransitionTierLowered},
		kinds(got[len(got)-2:]), "a reopened page notifies")
}

func TestJournalTransitionsAreBounded(t *testing.T) {
	var j Journal
	for i := 0; i < maxTransitions+5; i++ {
		j.add(Transition{At: at(time.Duration(i) * time.Second),
			Kind: TransitionRecovering})
	}
	require.Len(t, j.Transitions, maxTransitions)
	assert.Equal(t, at(5*time.Second), j.Transitions[0].At)
}

func TestJournalTransitionKindNamesEveryMove(t *testing.T) {
	tests := []struct {
		from, to State
		want     string
	}{
		{0, Settling, TransitionOpened},
		{Settling, Open, TransitionAnnounced},
		{Open, Recovering, TransitionRecovering},
		{Recovering, Open, TransitionFailingAgain},
		{Recovering, Flapping, TransitionFlapping},
		{Settling, Flapping, TransitionFlapping},
		{Open, Resolved, TransitionResolved},
		{Resolved, Open, TransitionReopened},
		{Resolved, Settling, TransitionReopened},
		{Flapping, Open, TransitionStateChanged},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, transitionKind(tt.from, tt.to),
			"%s to %s", tt.from, tt.to)
	}
}

func TestJournalStoresErrorKeyWhenAnnounced(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	web.Mode = detection.ModeCrashLoop
	r.raise(at(0), web)
	r.tick(at(time.Second))
	assert.Empty(t, r.only().Journal.ErrorKey, "settling")
	assert.NotEmpty(t, r.only().ErrorKey(), "computed while settling")

	r.tick(at(DefaultSettle))
	p := r.only()
	assert.Equal(t, SameErrorKey(p.Mode, ""), p.Journal.ErrorKey)

	// A cause found later does not move the stored key.
	r.m.mu.Lock()
	r.m.lookup(entity(kube.KindPod, "web")).Cause =
		&rootcause.CauseRecord{Rule: "rollout"}
	r.m.mu.Unlock()
	assert.Equal(t, p.Journal.ErrorKey, r.only().ErrorKey())
}

func TestSameErrorKeyIsEmptyWithoutModeOrRule(t *testing.T) {
	assert.Empty(t, SameErrorKey("", ""))
	assert.Equal(t, "CrashLoop\x00rollout",
		SameErrorKey(detection.ModeCrashLoop, "rollout"))
	old := Incident{Mode: detection.ModeCrashLoop}
	assert.Equal(t, "CrashLoop\x00", old.ErrorKey())
}

func TestJournalSurvivesARestartWithoutATransition(t *testing.T) {
	r, start := announcedRig(t)
	records := r.m.Export()
	body, err := json.Marshal(records)
	require.NoError(t, err)
	var back []Record
	require.NoError(t, json.Unmarshal(body, &back))
	require.Equal(t, records[0].Transitions, back[0].Transitions)
	assert.Equal(t, records[0].Level, back[0].Level)

	fresh := newRig(t, Config{})
	fresh.m.Restore(back, time.Time{})
	fresh.raise(start.Add(time.Minute), podSig("web"))
	fresh.tick(start.Add(time.Minute))
	got := fresh.only().Journal
	assert.Equal(t, kinds(records[0].Transitions), kinds(got.Transitions))
	assert.Equal(t, records[0].ErrorKey, got.ErrorKey)
}

func TestJournalOfAnOlderRecordStartsEmpty(t *testing.T) {
	r := newRig(t, Config{})
	r.m.Restore([]Record{{ID: "inc-1-7f3a",
		Root: entity(kube.KindPod, "web"), State: Open, Tier: Notify,
		Opened: at(0), Announced: at(time.Minute)}}, time.Time{})
	p := r.only()
	assert.Empty(t, p.Journal.Transitions)
	assert.Empty(t, p.Journal.Breaks)
	assert.Empty(t, p.Journal.Changes)
}

func TestJournalLevelSaysWhyItPages(t *testing.T) {
	r := newRig(t, Config{})
	dns := sig(entity(kube.KindDeployment, "coredns"),
		reasons.CoreDNSUnavailable, detection.Critical)
	r.raise(at(0), dns)
	r.tick(at(time.Second))
	p := r.only()
	assert.Equal(t, levelPagePrefix+"cluster-dns-failing", p.Journal.Level)
	assert.Equal(t, "Pages: cluster DNS is failing.",
		AlertLevelText(p.Journal.Level))
}
