package incident

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestSeverityTierMapping(t *testing.T) {
	cases := map[string]struct {
		in   string
		want Tier
		ok   bool
	}{
		"critical pages":     {"critical", Page, true},
		"case insensitive":   {" Critical ", Page, true},
		"high notifies":      {"high", Notify, true},
		"medium notifies":    {"medium", Notify, true},
		"warning notifies":   {"warning", Notify, true},
		"low digests":        {"low", Digest, true},
		"info digests":       {"info", Digest, true},
		"normal digests":     {"normal", Digest, true},
		"unknown is ignored": {"loud", Silent, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, ok := severityTier(c.in)
			if got != c.want || ok != c.ok {
				t.Fatalf("got %v,%v want %v,%v", got, ok, c.want, c.ok)
			}
		})
	}
}

func overrideIncident(
	rootKind inventory.Kind, reasons ...string,
) *Incident {
	root := inventory.CoreID(rootKind, "ns", "r")
	p := &Incident{Root: root, Members: map[detection.Key]detection.Finding{}}
	for _, r := range reasons {
		s := detection.Finding{Entity: root, Reason: r}
		p.Members[s.Key()] = s
	}
	return p
}

func TestOverridesReasonRaisesToPage(t *testing.T) {
	o := newOverrides(map[string]string{"oomkilled": "critical"}, nil)
	p := overrideIncident(kube.KindPod, "OOMKilled")
	if got := o.apply(p, Notify); got != Page {
		t.Fatalf("got %v want Page", got)
	}
}

func TestOverridesReasonLowersToDigest(t *testing.T) {
	o := newOverrides(map[string]string{"CrashLoop": "low"}, nil)
	p := overrideIncident(kube.KindPod, "crashloop")
	if got := o.apply(p, Page); got != Digest {
		t.Fatalf("got %v want Digest", got)
	}
}

func TestOverridesReasonTakesLoudestMember(t *testing.T) {
	o := newOverrides(map[string]string{
		"a": "low", "b": "critical",
	}, nil)
	p := overrideIncident(kube.KindPod, "a", "b", "c")
	if got := o.apply(p, Notify); got != Page {
		t.Fatalf("got %v want Page", got)
	}
}

func TestOverridesReasonBeatsOwnerKind(t *testing.T) {
	o := newOverrides(
		map[string]string{"a": "low"},
		map[string]string{"pod": "critical"})
	p := overrideIncident(kube.KindPod, "a")
	if got := o.apply(p, Notify); got != Digest {
		t.Fatalf("got %v want Digest", got)
	}
}

func TestOverridesOwnerKindMatchesRootAndImpact(t *testing.T) {
	o := newOverrides(nil, map[string]string{
		"StatefulSet": "critical", "Service": "low",
	})
	root := overrideIncident(kube.KindStatefulSet, "x")
	if got := o.apply(root, Notify); got != Page {
		t.Fatalf("root: got %v want Page", got)
	}
	impact := overrideIncident(kube.KindPod, "x")
	impact.Impact = []inventory.EntityID{
		inventory.CoreID(kube.KindStatefulSet, "ns", "db"),
		inventory.CoreID("service", "ns", "svc"),
	}
	if got := o.apply(impact, Digest); got != Page {
		t.Fatalf("impact: got %v want Page", got)
	}
}

func TestOverridesNoMatchKeepsBase(t *testing.T) {
	o := newOverrides(map[string]string{"a": "low"}, nil)
	p := overrideIncident(kube.KindPod, "other")
	if got := o.apply(p, Notify); got != Notify {
		t.Fatalf("got %v want Notify", got)
	}
}

func TestOverridesNoMembersNeverApply(t *testing.T) {
	o := newOverrides(nil, map[string]string{"pod": "critical"})
	p := overrideIncident(kube.KindPod)
	if got := o.apply(p, Notify); got != Notify {
		t.Fatalf("got %v want Notify", got)
	}
}

func TestCompileOverridesCaseCollisionIsDeterministic(t *testing.T) {
	in := map[string]string{"OOMKilled": "critical", "oomkilled": "low"}
	for i := 0; i < 50; i++ {
		got := compileOverrides(in)
		if got["oomkilled"] != Page {
			t.Fatalf("iteration %d: tier = %v, want first key wins", i,
				got["oomkilled"])
		}
	}
}
