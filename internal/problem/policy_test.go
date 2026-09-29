package problem

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

func problemOf(
	root knowledge.EntityID, impact []knowledge.EntityID,
	sigs ...signal.Signal,
) *Problem {
	p := &Problem{
		Root: root, Impact: impact, Tier: Digest,
		Members: map[signal.Key]signal.Signal{},
	}
	for _, s := range sigs {
		p.Members[s.Key()] = s
	}
	return p
}

func TestTierDerivation(t *testing.T) {
	node := entity(kube.KindNode, "n1")
	deploy := entity(kube.KindDeployment, "web")
	ingress := entity(kube.KindIngress, "web")
	crash := constant.ReasonCrashLoopBackOff
	cases := map[string]struct {
		p    *Problem
		want Tier
	}{
		"warning notifies": {
			problemOf(deploy, nil, sig(deploy, crash, signal.Warning)),
			Notify,
		},
		"critical without users notifies": {
			problemOf(deploy, nil, sig(deploy, crash, signal.Critical)),
			Notify,
		},
		"critical reaching ingress pages": {
			problemOf(deploy, []knowledge.EntityID{ingress},
				sig(deploy, crash, signal.Critical)),
			Page,
		},
		"critical node pages": {
			problemOf(node, nil, sig(node, "NodeNotReady", signal.Critical)),
			Page,
		},
		"info digests": {
			problemOf(deploy, nil, sig(deploy, crash, signal.Info)),
			Digest,
		},
		"digest reasons digest": {
			problemOf(deploy, nil, sig(deploy,
				constant.ReasonHPAMaxedOut, signal.Critical)),
			Digest,
		},
		"node draining digests": {
			problemOf(node, nil,
				sig(node, constant.ReasonNodeDraining, signal.Critical),
				sig(node, "NodeNotReady", signal.Critical)),
			Digest,
		},
		"empty keeps previous tier": {
			&Problem{Tier: Page, Members: nil}, Page,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := tier(c.p); got != c.want {
				t.Fatalf("tier = %v, want %v", got, c.want)
			}
		})
	}
}

func dailyOccurrences(hours ...int) []time.Time {
	var out []time.Time
	for day, h := range hours {
		out = append(out, time.Date(2026, 1, 1+day, h, 0, 0, 0, time.UTC))
	}
	return out
}

func TestRoutineNeedsThreeDaysAtTheSameTime(t *testing.T) {
	cases := map[string]struct {
		times []time.Time
		want  bool
	}{
		"same time three days":   {dailyOccurrences(3, 3, 3), true},
		"two days is not enough": {dailyOccurrences(3, 3), false},
		"different times":        {dailyOccurrences(3, 9, 15), false},
		"same day repeated": {[]time.Time{
			at(0), at(time.Minute), at(2 * time.Minute)}, false},
		"across midnight": {[]time.Time{
			time.Date(2026, 1, 1, 23, 50, 0, 0, time.UTC),
			time.Date(2026, 1, 2, 0, 10, 0, 0, time.UTC),
			time.Date(2026, 1, 3, 23, 55, 0, 0, time.UTC),
		}, true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			p := &Problem{Occurrences: c.times}
			if got := routine(p); got != c.want {
				t.Fatalf("routine = %v, want %v", got, c.want)
			}
		})
	}
}

func TestTierRoutineProblemGoesToDigest(t *testing.T) {
	deploy := entity(kube.KindDeployment, "web")
	p := problemOf(deploy, nil,
		sig(deploy, constant.ReasonCrashLoopBackOff, signal.Critical))
	p.Occurrences = dailyOccurrences(3, 3, 3)
	if got := tier(p); got != Digest {
		t.Fatalf("tier = %v, want Digest", got)
	}
}

func TestManagerRoutineRecurrenceIsDemotedToDigest(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	for day := 0; day < 3; day++ {
		start := at(time.Duration(day) * 24 * time.Hour)
		r.raise(start, web)
		ds := r.tick(start.Add(DefaultSettle))
		wantAction(t, ds, Announce, "settled")
		want := Notify
		if day == 2 {
			want = Digest
		}
		if ds[0].Problem.Tier != want {
			t.Fatalf("day %d tier = %v, want %v",
				day, ds[0].Problem.Tier, want)
		}
		r.clear(start.Add(2*time.Minute), web)
		r.tick(start.Add(2 * time.Minute))
		r.tick(start.Add(2*time.Minute + DefaultHold))
	}
	if got := r.only().Tier; got != Digest {
		t.Fatalf("tier = %v, want Digest", got)
	}
}

func TestManagerSeverityOverrideAppliesToTier(t *testing.T) {
	r := newRig(t, Config{
		SeverityByReason: map[string]string{"crashloopbackoff": "low"},
	})
	r.raise(at(0), podSig("web"))
	if got := r.only().Tier; got != Digest {
		t.Fatalf("tier = %v, want Digest", got)
	}
}
