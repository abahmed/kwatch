//go:build browser

package dashboard

import (
	"strings"
	"testing"
	"time"
)

// spotStatus is the healthy status with two restarts, the time kwatch
// was not watching, and optionally a spot node.
func spotStatus(spot string, gapSeconds, unknown int) map[string]any {
	st := healthStatus(healthOpts{memory: 0.4})
	self := st["self"].(map[string]any)
	self["restarts"] = []map[string]any{
		{"at": isoAgo(2 * time.Hour), "reason": "node_disruption",
			"stoppedAt": isoAgo(2*time.Hour + 4*time.Minute)},
		{"at": isoAgo(5 * time.Hour), "reason": "node_disruption"},
	}
	kw := st["kwatch"].(map[string]any)
	kw["restarts24h"] = 2
	kw["notWatchingSeconds"] = gapSeconds
	if unknown > 0 {
		kw["gapUnknown"] = unknown
	}
	if spot != "" {
		kw["spotNode"] = map[string]any{"label": spot}
	}
	return st
}

func seeSpot(t *testing.T, st map[string]any) string {
	t.Helper()
	srv := stubAPIServer(t, map[string]any{"/api/status": st})
	return healthText(t, srv, 1280, `(() => document.querySelector(
    "main, #app").textContent + "|" + document.querySelectorAll(
    "[data-spot]").length)()`)
}

// The kwatch page says how long kwatch was not watching, and says so
// for a spot node, in plain words and with no provider named.
func TestBrowserHealthShowsGapAndSpotNotice(t *testing.T) {
	page := seeSpot(t, spotStatus("karpenter.sh/capacity-type=spot", 300, 1))
	for _, want := range []string{
		"Not watching for about 5 min in the last 24 hours.",
		"1 restart has no recorded stop time, so it is not counted.",
		"kwatch runs on a spot node",
		"not watching for a few minutes",
		"priority class", "nodeSelector or affinity",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if !strings.HasSuffix(page, "|1") {
		t.Errorf("want one spot notice, got %q", page[len(page)-3:])
	}
	for _, bad := range []string{"Karpenter", "AWS", "GKE", "Azure"} {
		if strings.Contains(page, bad) {
			t.Errorf("the notice names a provider: %q", bad)
		}
	}
}

// No label, no notice; and no gap line when nothing was measured.
func TestBrowserHealthNoSpotNoticeWithoutLabel(t *testing.T) {
	page := seeSpot(t, spotStatus("", 0, 0))
	if strings.Contains(page, "spot node") || !strings.HasSuffix(page, "|0") {
		t.Errorf("a notice without a label: %q", page)
	}
	if strings.Contains(page, "Not watching for") {
		t.Errorf("a gap line without a gap")
	}
}

// The kwatch page names the cluster and where the name came from, or
// says once that the cluster has no name and how to set one.
func TestBrowserHealthShowsClusterNameAndNoNameNotice(t *testing.T) {
	st := spotStatus("", 0, 0)
	st["kwatch"].(map[string]any)["clusterName"] = map[string]any{
		"name": "prod-eu-1", "source": "configured"}
	page := seeSpot(t, st)
	if !strings.Contains(page, "Cluster name: prod-eu-1 (configured)") {
		t.Errorf("page lacks the cluster name line: %q", page)
	}
	if strings.Contains(page, "has no name") {
		t.Error("a named cluster shows the no-name notice")
	}

	st = spotStatus("", 0, 0)
	st["kwatch"].(map[string]any)["clusterName"] = map[string]any{
		"notice": "This cluster has no name. Set app.clusterName so " +
			"alerts and the dashboard say which cluster they are about."}
	page = seeSpot(t, st)
	for _, want := range []string{"This cluster has no name",
		"Set app.clusterName so alerts and the dashboard say which " +
			"cluster they are about."} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(page, "Cluster name:") {
		t.Error("an unnamed cluster shows a name line")
	}
}
