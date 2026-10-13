package selfcheck

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func day(n int) time.Time { return t0.Add(time.Duration(n) * 24 * time.Hour) }

func TestCountsSummarizeTheLastSevenDays(t *testing.T) {
	c := NewCounts()
	c.Record("rollout", day(0), CauseConfirmed)
	c.Record("rollout", day(1), CauseConfirmed)
	c.Record("rollout", day(2), OtherChanged)
	c.Record("rollout", day(3), NothingChanged)
	c.Record("node-pressure", day(3), NothingChanged)
	got := c.Summary(day(3))
	if len(got) != 2 {
		t.Fatalf("summary = %+v, want two rules", got)
	}
	if got[0].Rule != "node-pressure" || got[1].Rule != "rollout" {
		t.Errorf("rules not sorted: %+v", got)
	}
	r := got[1]
	if r.Confirmed != 2 || r.Other != 1 || r.None != 1 || r.Total() != 4 {
		t.Errorf("rollout = %+v", r)
	}
}

func TestCountsForgetDaysOlderThanTheWindow(t *testing.T) {
	c := NewCounts()
	c.Record("rollout", day(0), CauseConfirmed)
	c.Record("rollout", day(8), NothingChanged)
	got := c.Summary(day(8))
	if len(got) != 1 || got[0].Total() != 1 || got[0].Confirmed != 0 {
		t.Errorf("summary = %+v, want only the day-8 incident", got)
	}
	if c.Size() != 1 {
		t.Errorf("size = %d, old days must be dropped", c.Size())
	}
}

func TestCountsBoundTheRules(t *testing.T) {
	c := NewCounts()
	for i := 0; i < 3*MaxRules; i++ {
		c.Record(fmt.Sprintf("rule-%d", i), t0, NothingChanged)
	}
	if got := len(c.Summary(t0)); got > MaxRules+1 {
		t.Errorf("%d rules kept, at most %d plus the overflow row",
			got, MaxRules)
	}
	total := 0
	for _, r := range c.Summary(t0) {
		total += r.Total()
	}
	if total != 3*MaxRules {
		t.Errorf("total = %d, overflow must still be counted", total)
	}
}

func TestCountsRoundTripThroughJSON(t *testing.T) {
	c := NewCounts()
	c.Record("rollout", t0, CauseConfirmed)
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	back := NewCounts()
	if err := json.Unmarshal(data, back); err != nil {
		t.Fatal(err)
	}
	if got := back.Summary(t0); len(got) != 1 || got[0].Confirmed != 1 {
		t.Errorf("round trip = %+v", got)
	}
}

func TestCountsIgnoreGarbageOnLoad(t *testing.T) {
	back := NewCounts()
	err := json.Unmarshal([]byte(`{"rollout":{"not-a-day":[1,2,3]}}`), back)
	if err != nil || back.Size() != 0 {
		t.Errorf("err=%v size=%d, bad days must be skipped", err, back.Size())
	}
}

func TestSummaryLine(t *testing.T) {
	r := RuleSummary{Rule: "rollout", Confirmed: 3, Other: 1, None: 0}
	want := "cause confirmed by the fix in 3 of 4 resolved incidents " +
		"(last 7 days)"
	if got := r.Sentence(); got != want {
		t.Errorf("sentence = %q, want %q", got, want)
	}
	one := RuleSummary{Rule: "x", Confirmed: 1}
	if got := one.Sentence(); got != "cause confirmed by the fix in 1 of 1 "+
		"resolved incident (last 7 days)" {
		t.Errorf("singular = %q", got)
	}
}
