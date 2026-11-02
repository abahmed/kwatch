package dashboard

import (
	"fmt"
	"strings"
	"testing"
)

// insightHeavy is a detail whose insight parts alone are far over the
// response limit.
func insightHeavy() IncidentDetail {
	pad := strings.Repeat("z", 400)
	d := IncidentDetail{Waiting: &Waiting{Message: pad}}
	for i := range 200 {
		name := fmt.Sprint(i)
		d.Warnings = append(d.Warnings, RecentWarning{Reason: name,
			Message: pad})
		d.Traffic = append(d.Traffic, TrafficRow{Name: name,
			Backends: []string{pad}})
		d.Nodes = append(d.Nodes, NodeRow{Name: name,
			Conditions: []NodeCondition{{Type: "Ready", Message: pad}}})
		d.Probes = append(d.Probes, ProbeRow{Pod: name, LastMessage: pad})
		d.Waiting.Reasons = append(d.Waiting.Reasons, WaitReason{
			Text: pad, Reason: name})
	}
	return d
}

func TestFitDetailCutsInsightPartsToFit(t *testing.T) {
	d := insightHeavy()
	fitDetail(&d, detailBudget)
	if size := detailSize(&d); size > detailBudget {
		t.Fatalf("detail is %d bytes", size)
	}
	tr := d.Truncated
	if tr == nil || tr.Warnings == 0 || tr.Traffic == 0 || tr.Nodes == 0 ||
		tr.Probes == 0 || tr.WaitReasons == 0 || tr.Texts == 0 {
		t.Fatalf("truncated = %+v", tr)
	}
	if len(d.Warnings)+tr.Warnings != 200 ||
		len(d.Traffic)+tr.Traffic != 200 || len(d.Nodes)+tr.Nodes != 200 ||
		len(d.Probes)+tr.Probes != 200 ||
		len(d.Waiting.Reasons)+tr.WaitReasons != 200 {
		t.Errorf("kept and cut do not add up: %+v", tr)
	}
	if len(d.Warnings) > 0 && d.Warnings[0].Reason != "0" {
		t.Error("the newest warning was not kept")
	}
}

func TestFitDetailInsightIsDeterministic(t *testing.T) {
	const budget = 16 << 10
	a, b := insightHeavy(), insightHeavy()
	fitDetail(&a, budget)
	fitDetail(&b, budget)
	if detailSize(&a) > budget {
		t.Fatalf("detail is %d bytes", detailSize(&a))
	}
	if detailSize(&a) != detailSize(&b) || *a.Truncated != *b.Truncated {
		t.Error("two fits of the same detail differ")
	}
}

func TestFitDetailKeepsSmallInsightWhole(t *testing.T) {
	d := IncidentDetail{Warnings: []RecentWarning{{Reason: "BackOff"}}}
	fitDetail(&d, detailBudget)
	if d.Truncated != nil || len(d.Warnings) != 1 {
		t.Fatalf("a small detail was cut: %+v", d)
	}
}
