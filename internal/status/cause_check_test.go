package status

import (
	"encoding/json"
	"strings"
	"testing"
)

func checkedReport() Report {
	r := sampleReport()
	r.CauseChecks = []CauseCheck{
		{Rule: "rollout", Confirmed: 3, OtherChanged: 1, NothingChanged: 0,
			Total: 4, Text: "cause confirmed by the fix in 3 of 4 " +
				"resolved incidents (last 7 days)"},
		{Rule: "node-pressure", Confirmed: 0, NothingChanged: 2, Total: 2,
			Text: "cause confirmed by the fix in 0 of 2 resolved " +
				"incidents (last 7 days)"},
	}
	return r
}

func TestTextShowsTheCauseSelfCheckPerRule(t *testing.T) {
	text := checkedReport().Text()
	for _, want := range []string{
		"Cause self-check (display only)",
		"  rollout: cause confirmed by the fix in 3 of 4 resolved " +
			"incidents (last 7 days)",
		"  node-pressure: cause confirmed by the fix in 0 of 2",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

func TestTextOmitsTheCauseSelfCheckWithoutData(t *testing.T) {
	if strings.Contains(sampleReport().Text(), "self-check") {
		t.Error("an empty self-check must not add a section")
	}
}

func TestJSONCarriesTheCauseSelfCheck(t *testing.T) {
	data, err := json.Marshal(checkedReport())
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		CauseChecks []map[string]any `json:"causeChecks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.CauseChecks) != 2 {
		t.Fatalf("causeChecks = %v", got.CauseChecks)
	}
	first := got.CauseChecks[0]
	for _, key := range []string{"rule", "confirmed", "otherChanged",
		"nothingChanged", "total", "text"} {
		if _, ok := first[key]; !ok {
			t.Errorf("missing key %q in %v", key, first)
		}
	}
	empty, _ := json.Marshal(sampleReport())
	if strings.Contains(string(empty), "causeChecks") {
		t.Error("causeChecks must be omitted when empty")
	}
}
