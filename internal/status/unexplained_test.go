package status

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUnexplainedShowsCountsAndTopReasons(t *testing.T) {
	r := Report{At: testNow, Unexplained: Unexplained{
		Events: 7, Incidents: 1,
		Top: []UnexplainedRow{
			{Kind: "event", Mode: "UnusualEvent", Reason: "FailedThing",
				Count: 7, Sample: "the kubelet said no"},
			{Kind: "incident", Mode: "CrashLoop",
				Reason: "CrashLoopBackOff", Count: 1},
		},
	}}

	text := r.Text()

	for _, want := range []string{
		"Unexplained (last 7 days): 7 unknown events, 1 incident " +
			"with no clear cause",
		"  event UnusualEvent FailedThing: 7, e.g. " +
			"\"the kubelet said no\"",
		"  incident CrashLoop CrashLoopBackOff: 1",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

func TestUnexplainedIsInTheJSONReport(t *testing.T) {
	data, err := json.Marshal(Report{At: testNow,
		Unexplained: Unexplained{Events: 2}})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Unexplained struct {
			Events int `json:"events"`
		} `json:"unexplained"`
	}
	err = json.Unmarshal(data, &got)
	if err != nil || got.Unexplained.Events != 2 {
		t.Fatalf("unexplained = %+v, err %v", got, err)
	}
}
