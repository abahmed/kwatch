package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestDetailTimingNamesStartNoticeAndFirstSend(t *testing.T) {
	p := testIncident("a", "shop", 0)
	f := detection.Finding{Entity: p.Root, Reason: "CrashLoopBackOff",
		Since: testEpoch.Add(-36 * time.Second)}
	late := detection.Finding{Entity: p.Root, Reason: "Other",
		Since: testEpoch.Add(time.Minute)}
	risk := detection.Finding{Entity: p.Root, Reason: "Risk",
		Since: testEpoch.Add(-time.Hour), Advisory: true}
	p.Members = map[detection.Key]detection.Finding{f.Key(): f,
		late.Key(): late, risk.Key(): risk}
	s := newTestServer(p)
	s.SetDeliveryTrail(func(string) []DeliveryAttempt {
		return []DeliveryAttempt{
			{At: testEpoch.Add(-time.Hour), Channel: "email", Result: "ok"},
			{At: testEpoch.Add(10 * time.Second), Channel: "slack",
				Result: "failed"},
			{At: testEpoch.Add(29 * time.Second), Channel: "slack",
				Result: "ok"},
			{At: testEpoch.Add(40 * time.Second), Channel: "email",
				Result: "ok"},
		}
	})
	d := detailOf(t, s, "a")
	tm := d.Timing
	if tm == nil || !tm.StartedAt.Equal(f.Since) ||
		!tm.NoticedAt.Equal(testEpoch) ||
		!tm.ToldAt.Equal(testEpoch.Add(29*time.Second)) ||
		tm.ToldVia != "Slack" {
		t.Errorf("timing = %+v", tm)
	}
}

func TestDetailTimingWithoutFindingsOrSends(t *testing.T) {
	d := detailOf(t, newTestServer(testIncident("a", "shop", 0)), "a")
	if d.Timing == nil || !d.Timing.StartedAt.IsZero() ||
		!d.Timing.ToldAt.IsZero() || d.Timing.ToldVia != "" {
		t.Errorf("timing = %+v", d.Timing)
	}
	if got := timingOf(incident.Incident{}, nil); got != nil {
		t.Errorf("empty incident timing = %+v", got)
	}
	p := testIncident("b", "shop", 0)
	f := detection.Finding{Entity: p.Root, Since: testEpoch.Add(time.Hour)}
	p.Members = map[detection.Key]detection.Finding{f.Key(): f}
	if got := startedAt(p); !got.Equal(p.Opened) {
		t.Errorf("a start after the open = %v", got)
	}
}

func TestDetailSameTimeListsNearbyIssuesClosestFirst(t *testing.T) {
	a := testIncident("a", "shop", 0)
	shop := testIncident("b", "shop", 2*time.Minute)
	other := testIncident("c", "web", -4*time.Minute)
	far := testIncident("d", "shop", 10*time.Minute)
	quiet := testIncident("e", "shop", time.Minute)
	quiet.Tier = incident.Silent
	gone := testIncident("f", "web", -time.Minute)
	gone.State, gone.Tier = incident.Resolved, incident.Silent
	d := detailOf(t, newTestServer(a, shop, other, far, quiet, gone),
		"a")
	var got []string
	for _, s := range d.SameTime {
		got = append(got, s.ID+":"+s.Relation)
	}
	want := "b:same namespace f:cluster c:cluster"
	if strings.Join(got, " ") != want {
		t.Errorf("sameTime = %v, want %s", got, want)
	}
	if s := d.SameTime[0]; s.Title != "Deployment app-b in shop" ||
		s.Severity != "notify" || s.State != "open" ||
		s.Namespace != "shop" || !s.StartedAt.Equal(shop.Opened) {
		t.Errorf("first = %+v", s)
	}
}

func TestDetailSameTimeAbsentWhenAlone(t *testing.T) {
	d := detailOf(t, newTestServer(testIncident("a", "shop", 0),
		testIncident("b", "shop", time.Hour)), "a")
	if d.SameTime != nil {
		t.Errorf("sameTime = %+v", d.SameTime)
	}
}

func TestDetailSameTimeFindsTheSameNode(t *testing.T) {
	tm := newTestModel(t)
	node := inventory.CoreID(kube.KindNode, "", "n1")
	tm.observe(node, nil)
	root := func(ns string) inventory.EntityID {
		dep := inventory.CoreID(kube.KindDeployment, ns, "api")
		pod := inventory.CoreID(kube.KindPod, ns, "api-1")
		tm.observe(dep, nil)
		tm.observe(pod, nil)
		tm.relate(pod, inventory.OwnedBy, dep)
		tm.relate(pod, inventory.RunsOn, node)
		return dep
	}
	a := failingAt("a", root("shop"), testEpoch)
	b := failingAt("b", root("web"), testEpoch.Add(time.Minute))
	c := failingAt("c", inventory.CoreID(kube.KindDeployment, "shop",
		"db"), testEpoch.Add(30*time.Second))
	d := detailOf(t, contextServer(tm.m, false, a, b, c), "a")
	if len(d.SameTime) != 2 || d.SameTime[0].ID != "b" ||
		d.SameTime[0].Relation != relationNode ||
		d.SameTime[1].Relation != relationNamespace {
		t.Errorf("sameTime = %+v", d.SameTime)
	}
}

func TestDetailSameTimeIsBounded(t *testing.T) {
	items := []incident.Incident{testIncident("a", "shop", 0)}
	for i := range maxSameTime + 4 {
		items = append(items, testIncident(fmt.Sprintf("p%02d", i), "web",
			time.Duration(i)*time.Second))
	}
	d := detailOf(t, newTestServer(items...), "a")
	if len(d.SameTime) != maxSameTime || d.Truncated == nil ||
		d.Truncated.SameTime != 4 || d.SameTime[0].ID != "p00" {
		t.Errorf("sameTime %d truncated %+v", len(d.SameTime),
			d.Truncated)
	}
}

func TestFitDetailCutsSameTimeAndUsageHours(t *testing.T) {
	pad := strings.Repeat("y", 900)
	d := IncidentDetail{}
	for i := range 400 {
		d.SameTime = append(d.SameTime, SameTimeItem{ID: fmt.Sprint(i),
			Title: pad})
	}
	for i := range 100 {
		h := UsageHistory{Pod: fmt.Sprint(i), Metric: metricMemory}
		for j := range 24 {
			h.Peaks = append(h.Peaks, UsagePeak{
				Start: testEpoch.Add(time.Duration(j) * time.Hour),
				Value: int64(j)})
		}
		d.UsageHours = append(d.UsageHours, h)
	}
	fitDetail(&d, detailBudget)
	if size := detailSize(&d); size > detailBudget {
		t.Fatalf("detail is %d bytes", size)
	}
	tr := d.Truncated
	if tr == nil || tr.SameTime == 0 || tr.UsageHours == 0 ||
		len(d.SameTime)+tr.SameTime != 400 ||
		len(d.UsageHours)+tr.UsageHours != 100 {
		t.Fatalf("sameTime %d usage %d truncated %+v", len(d.SameTime),
			len(d.UsageHours), tr)
	}
}
