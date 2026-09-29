package notice

import "testing"

func TestTextRendersSections(t *testing.T) {
	cases := map[string]struct {
		m    Message
		want string
	}{
		"title only": {
			Message{Status: StatusCritical, Title: "web is down"},
			"🔴 web is down",
		},
		"no status marker": {
			Message{Title: "plain"}, "plain",
		},
		"story lines": {
			Message{Status: StatusWarning, Title: "t",
				Lines: []string{"one", "two"}},
			"🟠 t\none\ntwo",
		},
		"confidence": {
			Message{Status: StatusResolved, Title: "t",
				Confidence: "high"},
			"✅ t\nConfidence: high",
		},
		"output": {
			Message{Title: "t", Output: []string{"boom", "bang"}},
			"t\n\nLast output:\n  boom\n  bang",
		},
		"timeline": {
			Message{Title: "t", Timeline: []string{"12:00 started"}},
			"t\n\nTimeline (UTC):\n  12:00 started",
		},
		"steps with and without command": {
			Message{Title: "t", Steps: []Step{
				{Text: "look"},
				{Text: "restart", Command: "kubectl rollout", Mutating: true},
			}},
			"t\n\nNext steps:\n• look\n• restart\n  kubectl rollout",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := Text(c.m); got != c.want {
				t.Fatalf("Text =\n%q\nwant\n%q", got, c.want)
			}
		})
	}
}

func TestTextRendersEverySectionInOrder(t *testing.T) {
	m := Message{
		Status: StatusFlapping, Title: "web flapping",
		Lines: []string{"story"}, Confidence: "medium",
		Output:   []string{"out"},
		Timeline: []string{"event"},
		Steps:    []Step{{Text: "act", Command: "cmd"}},
	}
	want := "🔁 web flapping\nstory\nConfidence: medium\n" +
		"\nLast output:\n  out\n" +
		"\nTimeline (UTC):\n  event\n" +
		"\nNext steps:\n• act\n  cmd"
	if got := Text(m); got != want {
		t.Fatalf("Text =\n%q\nwant\n%q", got, want)
	}
}
