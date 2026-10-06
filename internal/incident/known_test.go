package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func heardAt(opened time.Time) Occurrence {
	return Occurrence{Opened: opened, Resolved: opened.Add(time.Minute),
		Heard: true, Mode: testMode}
}

// testMode is the failure mode the known-problem tests share.
const testMode detection.Mode = "CrashLoop"

func TestKnownCountsOnlyTheSameMode(t *testing.T) {
	now := at(0)
	old := heardAt(now.Add(-48 * time.Hour))
	recent := heardAt(now.Add(-time.Hour))
	other := old
	other.Mode = "OOMKilled"
	noMode := old
	noMode.Mode = ""
	cases := map[string]struct {
		mode    detection.Mode
		history []Occurrence
		want    bool
	}{
		"same mode":      {testMode, []Occurrence{old, recent}, true},
		"different mode": {"Pending", []Occurrence{old, recent}, false},
		"one other mode": {testMode, []Occurrence{other, recent}, false},
		"occurrence no mode": {testMode,
			[]Occurrence{noMode, recent}, false},
		"incident no mode": {"", []Occurrence{old, recent}, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			p := Incident{Mode: c.mode, History: c.history}
			if got := Known(p, now); got != c.want {
				t.Fatalf("Known = %v, want %v", got, c.want)
			}
		})
	}
}

func TestKnownNeedsTwoHeardOccurrencesADayApart(t *testing.T) {
	now := at(0)
	ago := func(d time.Duration) Occurrence { return heardAt(now.Add(-d)) }
	blip := Occurrence{Opened: now.Add(-3 * KnownAfter)}
	cases := map[string]struct {
		history []Occurrence
		want    bool
	}{
		"nothing heard": {nil, false},
		"one heard":     {[]Occurrence{ago(48 * time.Hour)}, false},
		"unheard blips": {[]Occurrence{blip, blip, blip}, false},
		"heard too soon": {
			[]Occurrence{ago(time.Hour), ago(2 * time.Hour)}, false},
		"just under a day": {[]Occurrence{
			ago(KnownAfter - time.Second), ago(time.Hour)}, false},
		"exactly a day": {[]Occurrence{
			ago(KnownAfter), ago(time.Hour)}, true},
		"blip ignored, two heard": {[]Occurrence{
			blip, ago(48 * time.Hour), ago(time.Hour)}, true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			p := Incident{Mode: testMode, History: c.history}
			if got := Known(p, now); got != c.want {
				t.Fatalf("Known = %v, want %v", got, c.want)
			}
		})
	}
}

// every spaces n occurrences by gaps, the last one at at(0).
func every(gaps ...time.Duration) []time.Time {
	out := []time.Time{at(0)}
	for i := len(gaps) - 1; i >= 0; i-- {
		out = append([]time.Time{out[0].Add(-gaps[i])}, out...)
	}
	return out
}

func TestRhythmDetectsRegularGaps(t *testing.T) {
	m := time.Minute
	p := Incident{Occurrences: every(40*m, 38*m, 43*m)}
	got, ok := Rhythm(p, at(0))
	if !ok || got < 39*m || got > 41*m {
		t.Fatalf("Rhythm = %v, %v; want about 40m", got, ok)
	}
}

func TestRhythmRejectsIrregularGaps(t *testing.T) {
	m := time.Minute
	p := Incident{Occurrences: every(10*m, 90*m, 20*m)}
	if got, ok := Rhythm(p, at(0)); ok {
		t.Fatalf("irregular gaps are no rhythm, got %v", got)
	}
}

func TestRhythmNeedsThreeOccurrencesWithinADay(t *testing.T) {
	m := time.Minute
	if _, ok := Rhythm(Incident{Occurrences: every(40 * m)}, at(0)); ok {
		t.Fatal("two occurrences cannot show a rhythm")
	}
	old := every(40*m, 40*m)
	for i := range old {
		old[i] = old[i].Add(-2 * RhythmWindow)
	}
	if _, ok := Rhythm(Incident{Occurrences: old}, at(0)); ok {
		t.Fatal("occurrences older than the window do not count")
	}
}

func knownIncident(
	root inventory.EntityID, tierNow Tier, reason string,
	sev detection.Severity,
) *Incident {
	p := incidentOf(root, nil, sig(root, reason, sev))
	p.Tier = tierNow
	p.Mode = testMode
	p.Opened = at(0)
	p.History = []Occurrence{heardAt(at(-48 * time.Hour)),
		heardAt(at(-time.Hour))}
	return p
}

var web = entity(kube.KindDeployment, "web")

func TestTierKnownNotifyGoesToDigest(t *testing.T) {
	p := knownIncident(web, Notify, reasons.CrashLoopBackOff,
		detection.Warning)
	if got := tier(p); got != Digest {
		t.Fatalf("tier = %v, want Digest", got)
	}
}

func TestTierRhythmicNotifyGoesToDigest(t *testing.T) {
	m := time.Minute
	p := knownIncident(web, Notify, reasons.CrashLoopBackOff,
		detection.Warning)
	p.History = nil
	p.Occurrences = every(40*m, 40*m)
	p.Opened = at(0)
	if got := tier(p); got != Digest {
		t.Fatalf("tier = %v, want Digest", got)
	}
}

func TestTierKnownPageStaysPage(t *testing.T) {
	node := entity(kube.KindNode, "n1")
	p := knownIncident(node, Page, "NodeNotReady",
		detection.Critical)
	if got := tier(p); got != Page {
		t.Fatalf("tier = %v, want Page", got)
	}
}

func TestIncidentModeIgnoresConfigurationRisks(t *testing.T) {
	root := inventory.EntityID{Kind: kube.KindDeployment, Name: "web"}
	pod := inventory.EntityID{Kind: kube.KindPod, Name: "web-1"}
	risk := detection.Finding{Entity: root,
		Reason: reasons.RiskNoMemoryLimit,
		Mode:   detection.ModeRiskNoMemoryLimit, Advisory: true}
	failure := detection.Finding{Entity: pod,
		Reason: reasons.ContainersNotReady,
		Mode:   detection.ModeNotReady}
	p := &Incident{Root: root, Members: map[detection.Key]detection.Finding{
		{Entity: root, Reason: risk.Reason}:   risk,
		{Entity: pod, Reason: failure.Reason}: failure,
	}}

	if got := incidentMode(p); got != detection.ModeNotReady {
		t.Fatalf("mode = %q, want the failure's, not the risk's", got)
	}

	delete(p.Members, detection.Key{Entity: pod, Reason: failure.Reason})
	if got := incidentMode(p); got != detection.ModeRiskNoMemoryLimit {
		t.Fatalf("risk-only incident mode = %q", got)
	}
}
