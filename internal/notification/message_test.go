package notification

import "testing"

func TestStatusMarkers(t *testing.T) {
	cases := map[string]struct {
		status Status
		emoji  string
		name   string
	}{
		"critical": {StatusCritical, "🔴", "critical"},
		"warning":  {StatusWarning, "🟠", "warning"},
		"flapping": {StatusFlapping, "🟠", "flapping"},
		"low":      {StatusLow, "🟡", "info"},
		"resolved": {StatusResolved, "✅", "resolved"},
		"unset":    {0, "", "info"},
		"unknown":  {99, "", "info"},
	}
	markers := map[string]bool{}
	for _, m := range Markers() {
		markers[m] = true
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := c.status.Emoji(); got != c.emoji {
				t.Errorf("Emoji = %q, want %q", got, c.emoji)
			}
			if got := c.status.Emoji(); got != "" && !markers[got] {
				t.Errorf("Emoji %q is not a composer marker", got)
			}
			if got := c.status.String(); got != c.name {
				t.Errorf("String = %q, want %q", got, c.name)
			}
		})
	}
}

func TestMessageIsOpening(t *testing.T) {
	cases := []struct {
		name string
		m    Message
		want bool
	}{
		{"first revision", Message{Revision: 1}, true},
		{"flagged later revision", Message{Revision: 4, Opens: true}, true},
		{"update", Message{Revision: 2}, false},
		{"resolve", Message{Revision: 1, Status: StatusResolved}, false},
	}
	for _, tc := range cases {
		if got := tc.m.IsOpening(); got != tc.want {
			t.Errorf("%s: IsOpening = %v, want %v", tc.name, got, tc.want)
		}
	}
}
