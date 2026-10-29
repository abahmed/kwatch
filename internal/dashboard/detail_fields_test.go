package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/status"
)

func TestDetailCarriesStepsKindsAndWorkloadKind(t *testing.T) {
	p := testIncident("a", "shop", 0)
	p.Steps = []incident.FixStep{
		{Text: "Give it more memory."},
		{Text: "Raise the limit:",
			Command: "kubectl -n shop set resources deploy/app-a " +
				"--limits=memory=512Mi"},
		{Command: "kubectl -n shop logs deploy/app-a password=hunter2"},
	}
	at := testEpoch.Add(time.Minute)
	p.Timeline = []incident.Event{
		{At: at, Text: "failing again: 3 times in the last hour"},
		{At: at, Text: "resolved: " + string(incident.ReasonSuperseded)},
		{At: at, Text: "something new"},
	}
	var d IncidentDetail
	body := get(t, newTestServer(p), "/api/incidents/a").Body.Bytes()
	if err := json.Unmarshal(body, &d); err != nil {
		t.Fatal(err)
	}
	if d.WorkloadKind != "Deployment" {
		t.Errorf("workloadKind = %q", d.WorkloadKind)
	}
	if len(d.FixSteps) != 3 || len(d.FixCommands) != 2 ||
		d.FixCommands[0] != p.Steps[1].Command {
		t.Errorf("steps %+v commands %+v", d.FixSteps, d.FixCommands)
	}
	if d.SuggestedFix != "Give it more memory.\nRaise the limit:" {
		t.Errorf("suggestedFix = %q", d.SuggestedFix)
	}
	if d.FixCommands[1] == p.Steps[2].Command {
		t.Errorf("a command with a secret was not redacted: %q",
			d.FixCommands[1])
	}
	kinds := []string{d.Timeline[0].Kind, d.Timeline[1].Kind,
		d.Timeline[1].Reason, d.Timeline[2].Kind}
	want := []string{"repeated", "resolved", "superseded", "note"}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("timeline kinds = %v, want %v", kinds, want)
			break
		}
	}
}

func TestEventKindsCoverEveryTimelineNote(t *testing.T) {
	cases := map[string][2]string{
		"failing again: 4 times in 1h":        {"repeated", ""},
		"happened again (3rd time this week)": {"repeated", ""},
		"acknowledged on Deployment shop/a":   {"acknowledged", ""},
		"acknowledgement removed":             {"ack_removed", ""},
		"cause revised: now explained by x":   {"cause_revised", ""},
		"flapping: 3 recoveries in 30m0s":     {"flapping", ""},
		"still failing: paging again":         {"paged_again", ""},
		"announced with a page for the same moment": {
			"paged_together", ""},
		"same outage as Node a, already paged": {"same_outage", ""},
		"resolved: healthy for 5m0s":           {"resolved", "recovered"},
		"resolved: stable for 30m0s":           {"resolved", "recovered"},
		"resolved: " + string(incident.ReasonRecoveredUnheard): {
			"resolved", "recovered_unheard"},
		"resolved: reopened and recovered before it was reported": {
			"resolved", "recovered_unheard"},
		"resolved: " + incident.StoppedTrackingPrefix + "1h": {
			"resolved", "stopped_tracking"},
		"resolved: something else": {"resolved", "other"},
	}
	for text, want := range cases {
		kind, reason := classifyEvent(text)
		if kind != want[0] || reason != want[1] {
			t.Errorf("%q = %s/%s, want %s/%s", text, kind, reason,
				want[0], want[1])
		}
	}
}

func TestStatusScanHealthFollowsTheEngineLoop(t *testing.T) {
	s := newSessionServer(false, testPassword)
	s.SetStatus(func() status.Report { return status.Report{} })
	read := func() StatusResponse {
		var out StatusResponse
		req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		w := s.do(req)
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	if r := read(); !r.LastScan.IsZero() || r.LastScanLate {
		t.Errorf("before the first loop: %+v", r)
	}
	s.NoteScan()
	if r := read(); !r.LastScan.Equal(testEpoch) || r.LastScanLate ||
		r.ScanEverySeconds != 10 {
		t.Errorf("fresh scan: %+v", r)
	}
	s.now = testEpoch.Add(3 * time.Minute)
	if r := read(); !r.LastScanLate {
		t.Errorf("scan 3 min old is not late: %+v", r)
	}
}

func TestChannelNamesUseTheProductSpelling(t *testing.T) {
	cases := map[string]string{"slack": "Slack", "teams": "Microsoft Teams",
		"pagerduty": "PagerDuty", "email": "Email", "": ""}
	for key, want := range cases {
		if got := channelName(key); got != want {
			t.Errorf("channelName(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestListAndDetailCarryTheRedactedCause(t *testing.T) {
	p := testIncident("a", "shop", 0)
	p.Cause = &rootcause.CauseRecord{
		Summary: "the database is down, password=hunter2"}
	var list ListResponse
	body := get(t, newTestServer(p), "/api/incidents?state=active").Body.Bytes()
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Incidents) != 1 {
		t.Fatalf("incidents = %+v", list.Incidents)
	}
	cause := list.Incidents[0].RootCause
	if cause == "" || cause == p.Cause.Summary {
		t.Errorf("list rootCause = %q, want the redacted cause", cause)
	}
	var d IncidentDetail
	body = get(t, newTestServer(p), "/api/incidents/a").Body.Bytes()
	if err := json.Unmarshal(body, &d); err != nil {
		t.Fatal(err)
	}
	if d.RootCause != cause {
		t.Errorf("detail rootCause = %q, list has %q", d.RootCause, cause)
	}
}

// Repeats kwatch folded into one timeline event reach the API, so the
// dashboard counts every occurrence.
func TestDetailTimelineCarriesFoldedRepeats(t *testing.T) {
	p := testIncident("a", "shop", 0)
	p.Timeline = []incident.Event{{At: testEpoch.Add(time.Minute),
		Text: "failing again: 3 times in the last hour", Repeats: 4}}
	var d IncidentDetail
	body := get(t, newTestServer(p), "/api/incidents/a").Body.Bytes()
	if err := json.Unmarshal(body, &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Timeline) != 1 || d.Timeline[0].Repeats != 4 {
		t.Errorf("timeline = %+v, want one event with 4 repeats",
			d.Timeline)
	}
}
