package notification

import "testing"

func TestTextRendersPlainSentences(t *testing.T) {
	cases := map[string]struct {
		m    Message
		want string
	}{
		"note wins": {
			Message{Status: StatusCritical, Title: "web is down",
				Note: "🔴 web is down in shop. It crashed."},
			"🔴 web is down in shop. It crashed.",
		},
		"title only": {
			Message{Status: StatusCritical, Title: "web is down"},
			"🔴 web is down",
		},
		"no status marker": {
			Message{Title: "plain"}, "plain",
		},
		"marker field wins over status": {
			Message{Status: StatusFlapping, Marker: MarkerPage,
				Title: "web keeps failing"},
			"🔴 web keeps failing",
		},
		"low status": {
			Message{Status: StatusLow, Title: "web is slow"},
			"🟡 web is slow",
		},
		"story lines": {
			Message{Status: StatusWarning, Title: "t",
				Lines: []string{"one", "two."}},
			"🟠 t. one. two.",
		},
		"no labelled sections": {
			Message{Status: StatusResolved, Title: "t",
				Confidence: "high", Output: []string{"boom"},
				Timeline: []string{"12:00 started"}},
			"✅ t",
		},
		"first step with its command": {
			Message{Title: "web is down", Steps: []Step{
				{Text: "Restart it.", Command: "kubectl rollout restart"},
				{Text: "look"},
			}},
			"web is down. Restart it: kubectl rollout restart",
		},
		"step without command": {
			Message{Title: "t", Steps: []Step{{Text: "look"}}},
			"t. look.",
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
