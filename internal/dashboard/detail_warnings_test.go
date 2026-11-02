package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

const readinessFailed = "Readiness probe failed: HTTP probe failed " +
	"with statuscode: 500"

func TestDetailWarningsFoldByReasonAndMessage(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	api := tm.container(pod, "api", nil)
	pod2 := tm.pod(root, "api-2", nil)
	api2 := tm.container(pod2, "api", nil)
	tm.note(api, "Unhealthy", readinessFailed, 200, time.Minute)
	tm.note(api2, "Unhealthy", readinessFailed, 14, 2*time.Minute)
	tm.note(pod, "BackOff", "Back-off restarting failed container", 5,
		5*time.Minute)
	tm.note(pod, "Failed", "pull with Authorization: Bearer "+
		"abcdefghijklmnop1234", 1, 3*time.Minute)
	tm.noteAt(pod, inventory.Note{Reason: "Pulled", Message: "pulled",
		Count: 1, At: testEpoch.Add(-time.Minute), Source: "kubelet"})
	tm.note(pod2, "Old", "before it started", 1, 2*time.Hour)

	d := detailOf(t, contextServer(tm.m, false,
		failingAt("w", root, testEpoch.Add(-time.Hour))), "w")
	if len(d.Warnings) != 3 {
		t.Fatalf("warnings = %+v", d.Warnings)
	}
	w := d.Warnings[0]
	if w.Reason != "Unhealthy" || w.Message != readinessFailed ||
		w.Count != 214 || w.Objects != 2 ||
		!w.LastSeen.Equal(testEpoch.Add(-time.Minute)) ||
		w.Object != "container shop/api-1/api" {
		t.Errorf("folded warning = %+v", w)
	}
	if d.Warnings[1].Reason != "Failed" ||
		strings.Contains(d.Warnings[1].Message, "abcdefghijklmnop1234") {
		t.Errorf("secret kept or wrong order: %+v", d.Warnings[1])
	}
	if d.Warnings[2].Reason != "BackOff" || d.Warnings[2].Count != 5 {
		t.Errorf("third = %+v", d.Warnings[2])
	}
	if d.Truncated != nil && d.Truncated.Warnings != 0 {
		t.Errorf("truncated = %+v", d.Truncated)
	}
}

func TestDetailWarningsAreBoundedNewestFirst(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	for i := range maxWarnings + 2 {
		tm.note(pod, fmt.Sprintf("R%02d", i), "m", 1,
			time.Duration(i+1)*time.Minute)
	}
	d := detailOf(t, contextServer(tm.m, false,
		failingAt("w", root, testEpoch.Add(-time.Hour))), "w")
	if len(d.Warnings) != maxWarnings || d.Warnings[0].Reason != "R00" ||
		d.Truncated == nil || d.Truncated.Warnings != 2 {
		t.Fatalf("warnings = %d, first %+v, truncated %+v",
			len(d.Warnings), d.Warnings[0], d.Truncated)
	}
}

func TestDetailWarningsAbsentWithoutEvents(t *testing.T) {
	tm := newTestModel(t)
	root, _ := tm.deployment()
	s := contextServer(tm.m, false,
		failingAt("w", root, testEpoch.Add(-time.Hour)))
	body := get(t, s, "/api/incidents/w").Body.String()
	for _, field := range []string{`"warnings"`, `"traffic"`, `"nodes"`,
		`"probes"`, `"waiting"`, `"links"`} {
		if strings.Contains(body, field) {
			t.Errorf("detail without data has %s: %s", field, body)
		}
	}
	body = get(t, newTestServer(testIncident("a", "shop", -time.Hour)),
		"/api/incidents/a").Body.String()
	if strings.Contains(body, `"warnings"`) {
		t.Errorf("detail without a model has warnings: %s", body)
	}
}
