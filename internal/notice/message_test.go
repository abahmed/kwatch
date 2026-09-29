package notice

import "testing"

func TestStatusMarkers(t *testing.T) {
	cases := map[string]struct {
		status Status
		emoji  string
		name   string
	}{
		"critical": {StatusCritical, "🔴", "critical"},
		"warning":  {StatusWarning, "🟠", "warning"},
		"flapping": {StatusFlapping, "🔁", "flapping"},
		"resolved": {StatusResolved, "✅", "resolved"},
		"unset":    {0, "", "info"},
		"unknown":  {99, "", "info"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := c.status.Emoji(); got != c.emoji {
				t.Errorf("Emoji = %q, want %q", got, c.emoji)
			}
			if got := c.status.String(); got != c.name {
				t.Errorf("String = %q, want %q", got, c.name)
			}
		})
	}
}
