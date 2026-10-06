package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func incidentOf(
	root inventory.EntityID, impact []inventory.EntityID,
	sigs ...detection.Finding,
) *Incident {
	p := &Incident{
		Root: root, Impact: impact, Tier: Digest,
		Members: map[detection.Key]detection.Finding{},
	}
	for _, s := range sigs {
		p.Members[s.Key()] = s
	}
	return p
}

// lostTraffic marks p as having a routed Service without healthy
// backends, as refresh would after reading the model.
func lostTraffic(p *Incident) *Incident {
	p.trafficLost = true
	return p
}

func TestTierDerivation(t *testing.T) {
	node := entity(kube.KindNode, "n1")
	deploy := entity(kube.KindDeployment, "web")
	ingress := entity(kube.KindIngress, "web")
	webhook := entity(kube.KindValidatingHook, "policy")
	route := entity("httproute", "web")
	crash := reasons.CrashLoopBackOff
	cases := map[string]struct {
		p    *Incident
		want Tier
	}{
		"a node that only stalls waits for the digest": {
			incidentOf(node, nil,
				sig(node, reasons.NodePSIHigh, detection.Warning)),
			Digest,
		},
		"critical throttling alone waits for the digest": {
			lostTraffic(incidentOf(deploy, []inventory.EntityID{ingress},
				sig(deploy, reasons.ContainerCPUThrottled,
					detection.Critical))),
			Digest,
		},
		"critical throttling does not lift a warning to a page": {
			lostTraffic(incidentOf(deploy, []inventory.EntityID{ingress},
				sig(deploy, reasons.ContainerCPUThrottled,
					detection.Critical),
				sig(deploy, crash, detection.Warning))),
			Notify,
		},
		"a risk never raises the tier": {
			incidentOf(deploy, nil, detection.Finding{Entity: deploy,
				Reason: reasons.RiskSingleReplica, Advisory: true,
				Severity: detection.Critical}),
			Digest,
		},
		"warning notifies": {
			incidentOf(deploy, nil, sig(deploy, crash, detection.Warning)),
			Notify,
		},
		"critical without users notifies": {
			incidentOf(deploy, nil, sig(deploy, crash, detection.Critical)),
			Notify,
		},
		"critical reaching ingress that lost traffic pages": {
			lostTraffic(incidentOf(deploy, []inventory.EntityID{ingress},
				sig(deploy, crash, detection.Critical))),
			Page,
		},
		"critical behind ingress with healthy backends notifies": {
			incidentOf(deploy, []inventory.EntityID{ingress},
				sig(deploy, crash, detection.Critical)),
			Notify,
		},
		"lost node pages": {
			incidentOf(node, nil, sig(node, "NodeNotReady", detection.Critical)),
			Page,
		},
		"node under pressure notifies": {
			incidentOf(node, nil, sig(node, reasons.NodeMemoryPressure,
				detection.Critical)),
			Notify,
		},
		"zone of lost nodes pages": {
			incidentOf(entity(kube.KindZone, "zone-b"), nil,
				sig(node, reasons.NodeHeartbeatStale, detection.Critical)),
			Page,
		},
		"info digests": {
			incidentOf(deploy, nil, sig(deploy, crash, detection.Info)),
			Digest,
		},
		"digest reasons digest": {
			incidentOf(deploy, nil, sig(deploy,
				reasons.HPAMaxedOut, detection.Critical)),
			Digest,
		},
		"drain within its envelope is silent": {
			incidentOf(node, nil,
				sig(node, reasons.NodeDraining, detection.Critical),
				sig(node, "NodeNotReady", detection.Critical),
				sig(deploy, crash, detection.Info)),
			Silent,
		},
		"drain whose pods stay unready notifies": {
			incidentOf(node, nil,
				sig(node, reasons.NodeDraining, detection.Info),
				sig(deploy, "PodNotReady", detection.Critical)),
			Notify,
		},
		"drain with only digest findings digests": {
			incidentOf(node, nil,
				sig(node, reasons.NodeDraining, detection.Info),
				sig(deploy, reasons.PodStuckTerminating,
					detection.Warning)),
			Digest,
		},
		"blocking webhook pages": {
			incidentOf(webhook, nil, sig(webhook,
				reasons.WebhookNoEndpoints, detection.Critical)),
			Page,
		},
		"ignoring webhook notifies": {
			incidentOf(webhook, nil,
				sig(webhook, reasons.WebhookNoEndpoints,
					detection.Warning),
				sig(deploy, crash, detection.Critical)),
			Notify,
		},
		"route losing its backends pages": {
			lostTraffic(incidentOf(deploy, []inventory.EntityID{route},
				sig(deploy, crash, detection.Critical))),
			Page,
		},
		"empty keeps previous tier": {
			&Incident{Tier: Page, Members: nil}, Page,
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

// TestPageRulesNameClusterWideRoots checks every cluster service that
// everyone depends on pages under its own named rule.
func TestPageRulesNameClusterWideRoots(t *testing.T) {
	cases := map[string]struct {
		root   inventory.EntityID
		reason string
	}{
		"cluster-dns-failing": {kube.ClusterDNS, reasons.CoreDNSUnavailable},
		"api-unavailable":     {kube.Etcd, reasons.EtcdUnavailable},
		"scheduler-down": {kube.Scheduler,
			reasons.SchedulerUnavailable},
		"controller-manager-down": {kube.ControllerManager,
			reasons.ControllerManagerUnavailable},
	}
	for want, c := range cases {
		t.Run(want, func(t *testing.T) {
			p := incidentOf(c.root, nil,
				sig(c.root, c.reason, detection.Critical))
			if got := pageRuleOf(p); got != want {
				t.Fatalf("rule = %q, want %q", got, want)
			}
			if got := tier(p); got != Page {
				t.Fatalf("tier = %v, want page", got)
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
			p := &Incident{Occurrences: c.times}
			if got := routine(p); got != c.want {
				t.Fatalf("routine = %v, want %v", got, c.want)
			}
		})
	}
}

func TestTierRoutineIncidentGoesToDigest(t *testing.T) {
	deploy := entity(kube.KindDeployment, "web")
	p := incidentOf(deploy, nil,
		sig(deploy, reasons.CrashLoopBackOff, detection.Critical))
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
		if ds[0].Incident.Tier != want {
			t.Fatalf("day %d tier = %v, want %v",
				day, ds[0].Incident.Tier, want)
		}
		r.clear(start.Add(2*time.Minute), web)
		r.tick(start.Add(2 * time.Minute))
		r.tick(start.Add(2*time.Minute + DefaultHold))
	}
	if got := r.of(web.Entity).Tier; got != Digest {
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

// A reopened page is held at notify but it paged: a rhythm or an old
// age must not drop it to the digest, which would leave its alert open.
func TestTierKeepsAHeldPageOutOfTheDigest(t *testing.T) {
	deploy := entity(kube.KindDeployment, "web")
	p := incidentOf(deploy, nil,
		sig(deploy, reasons.CrashLoopBackOff, detection.Critical))
	p.Opened = at(3 * 24 * time.Hour)
	p.Mode = detection.ModeCrashLoop
	p.History = []Occurrence{
		{Opened: at(0), Mode: p.Mode, Heard: true},
		{Opened: at(24 * time.Hour), Mode: p.Mode, Heard: true},
	}
	if got := tier(p); got != Digest {
		t.Fatalf("setup: tier = %v, want Digest for a known problem", got)
	}
	p.Delivery.HoldAtNotify()
	if got := tier(p); got == Digest {
		t.Fatalf("tier = %v, a held page must not be demoted", got)
	}
	p.Delivery = Delivery{}
	p.Delivery.MarkPaged()
	if got := tier(p); got == Digest {
		t.Fatalf("tier = %v, a paged incident must not be demoted", got)
	}
}
