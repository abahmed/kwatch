package reason

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestDescribeFieldsRendersChange(t *testing.T) {
	cases := []struct {
		name   string
		change knowledge.Change
		want   string
	}{
		{"no fields", knowledge.Change{}, "changed"},
		{"before and after", knowledge.Change{Fields: []knowledge.FieldChange{
			field("containers[app].image", "a:1", "a:2")}},
			"changed image a:1 → a:2"},
		{"created value", knowledge.Change{Fields: []knowledge.FieldChange{
			field("data.KEY", "", "x")}}, "changed KEY none → x"},
		{"path only", knowledge.Change{Fields: []knowledge.FieldChange{
			field("spec", "", "")}}, "changed spec"},
		{"extra fields", knowledge.Change{Fields: []knowledge.FieldChange{
			field("data.A", "1", "2"), field("data.B", "1", "2"),
			field("data.C", "1", "2")}}, "changed A 1 → 2 (+2 more)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeFields(tc.change); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestLastSegmentSplitsOnDotAndBracket(t *testing.T) {
	for in, want := range map[string]string{
		"a.b": "b", "containers[x]": "containers[x]", "plain": "plain",
		"trailing.": "trailing.",
	} {
		if got := lastSegment(in); got != want {
			t.Errorf("lastSegment(%q) = %q want %q", in, got, want)
		}
	}
}

func TestEvidenceHelpersReadSignal(t *testing.T) {
	s := signal.Signal{Summary: "boom", Evidence: []signal.Evidence{
		{Label: "scheduler", Value: "v1"}, {Label: "x", Value: "v2"},
	}}
	if got := evidenceText(s); got != "boom v1 v2" {
		t.Errorf("text = %q", got)
	}
	if evidenceValue(s, "x") != "v2" || evidenceValue(s, "no") != "" {
		t.Error("evidenceValue lookup wrong")
	}
	if !containsFold("Hello World", "WORLD") {
		t.Error("containsFold should ignore case")
	}
	if short(90*time.Second) == "" {
		t.Error("short renders a duration")
	}
}

func TestScorerClampsAndOrdersPoints(t *testing.T) {
	s := newScorer(0.9)
	s.support(0.05, "small")
	s.support(0.5, "big")
	s.contradict(0.1, "mid")
	score, points := s.result()
	if score != 1 {
		t.Errorf("score not clamped: %v", score)
	}
	if points[0].Text != "big" || points[2].Text != "small" {
		t.Errorf("points not sorted by weight: %v", points)
	}
	low := newScorer(0.1)
	low.contradict(0.5, "no")
	if got, _ := low.result(); got != 0 {
		t.Errorf("score not floored: %v", got)
	}
}
