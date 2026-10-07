package announce

import (
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
)

// Only the cause update and the resolve of an incident that fell from a
// thread leave the digest; its other updates ride in it.
func TestThreadNewsIsTheCauseAndTheResolve(t *testing.T) {
	cases := []struct {
		name string
		d    incident.Decision
		want bool
	}{
		{"cause", incident.Decision{Action: incident.Update,
			Reason: incident.ReasonCauseRevised, Thread: true}, true},
		{"resolve", incident.Decision{Action: incident.Resolve,
			Thread: true}, true},
		{"reminder", incident.Decision{Action: incident.Update,
			Reason: incident.ReasonReminder, Thread: true}, false},
		{"no thread", incident.Decision{Action: incident.Update,
			Reason: incident.ReasonCauseRevised}, false},
	}
	for _, c := range cases {
		if got := threadNews(c.d); got != c.want {
			t.Errorf("%s: threadNews = %v, want %v", c.name, got, c.want)
		}
	}
}
