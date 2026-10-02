package scenarios

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/scorecard"
)

func TestWrongHighGateNeedsEnoughCases(t *testing.T) {
	cases := map[string]struct {
		high, wrong int
		pass        bool
		value       string
	}{
		"too few to gate":     {10, 5, true, "not gated"},
		"within the limit":    {20, 1, true, "5%"},
		"above the limit":     {20, 2, false, "10%"},
		"no high confidence":  {0, 0, true, "not gated"},
		"many cases, all ok":  {40, 0, true, "0%"},
		"many cases, too bad": {40, 3, false, "7.5%"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			// The wrong cases are split across both sets, so the
			// gate must count held-out cases to see them all.
			var labelled, heldout []scorecard.Case
			for i := range c.high {
				one := scorecard.Case{
					Correct: i >= c.wrong, Confidence: 0.9,
				}
				if i%2 == 0 {
					labelled = append(labelled, one)
				} else {
					heldout = append(heldout, one)
				}
			}
			gate := wrongHighGate(labelled, heldout)
			if gate.Pass != c.pass ||
				!strings.HasPrefix(gate.Value, c.value) {
				t.Fatalf("gate = %+v", gate)
			}
		})
	}
}

func TestFirstMessageGateIsOnTheSlowest(t *testing.T) {
	delays := []time.Duration{20 * time.Second, 40 * time.Second,
		70 * time.Second}
	gate := firstMessageGate(incident.Page, delays, time.Minute)
	if gate.Pass || !strings.Contains(gate.Value, "max 1m10s") {
		t.Fatalf("gate = %+v, want a miss on the 70s case", gate)
	}
	gate = firstMessageGate(incident.Notify, delays[:2], time.Minute)
	if !gate.Pass {
		t.Fatalf("gate = %+v, want a pass", gate)
	}
	if gate = firstMessageGate(incident.Page, nil, time.Minute); !gate.Pass ||
		gate.Value != "no cases" {
		t.Fatalf("gate = %+v, want no cases", gate)
	}
}

func TestFailureStartIgnoresHistoryBeforeTheLog(t *testing.T) {
	floor := scenarioStart
	p := incident.Incident{Opened: floor.Add(3 * time.Minute)}
	if got := failureStart(p, floor); !got.Equal(p.Opened) {
		t.Fatalf("no findings: start = %s, want opened", got)
	}
	p.Members = map[detection.Key]detection.Finding{
		{Reason: "a"}: {Since: floor.Add(-time.Hour)},
		{Reason: "b"}: {Since: floor.Add(time.Minute)},
	}
	if got := failureStart(p, floor); !got.Equal(floor) {
		t.Fatalf("start = %s, want the log start", got)
	}
}

func TestNonEventAttribution(t *testing.T) {
	pod := func(ns string) inventory.EntityID {
		return inventory.EntityID{Kind: "pod", Namespace: ns}
	}
	node := func(name string) inventory.EntityID {
		return inventory.EntityID{Kind: "node", Name: name}
	}
	cases := map[inventory.EntityID]bool{
		pod("staging"): true, pod("calm-c3"): true,
		node("calm-n3-c2"): true, node("bg-1"): true,
		pod("shop-s4"): false, node("n1-s4"): false,
		{Kind: "cluster-dns", Name: "cluster-dns"}: false,
	}
	for id, want := range cases {
		if got := isNonEvent(id); got != want {
			t.Errorf("isNonEvent(%s) = %v, want %v", id, got, want)
		}
	}
	decisions := []incident.Decision{
		{Incident: incident.Incident{Tier: incident.Digest,
			Root: inventory.EntityID{Kind: "node", Name: "calm-n3"}}},
		{Incident: incident.Incident{Tier: incident.Notify,
			Root: inventory.EntityID{Kind: "node", Name: "n1"},
			Impact: []inventory.EntityID{
				{Kind: "pod", Namespace: "calm-c1"}}}},
		{Incident: incident.Incident{Tier: incident.Notify,
			Root: inventory.EntityID{Kind: "node", Name: "n1"}}},
	}
	if all, loud := nonEventNotifications(decisions); all != 2 || loud != 1 {
		t.Fatalf("non-events = %d (%d loud), want 2 (1 loud)", all, loud)
	}
}
