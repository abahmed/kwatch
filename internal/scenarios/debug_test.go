package scenarios

import (
	"flag"
	"sort"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

var only = flag.String("scenario", "",
	"TestScenarioReplay: replay only this scenario and print its messages")

// TestScenarioReplay replays each committed scenario and logs its verdict.
// It never fails: the gates judge. With -scenario it also prints every
// message, which is how a scenario is checked while it is written:
//
//	go test ./internal/scenarios -run TestScenarioReplay -v \
//		-scenario bad-rollout
func TestScenarioReplay(t *testing.T) {
	t.Parallel()
	// Held-out scenarios are replayed too when one is named, so any
	// scenario can be read while it is written.
	scenarios := library()
	heldout := map[string]bool{}
	if *only != "" {
		for _, s := range heldoutLibrary() {
			heldout[s.expect.Name] = true
			scenarios = append(scenarios, s)
		}
	}
	for _, s := range scenarios {
		name := s.expect.Name
		if *only != "" && name != *only {
			continue
		}
		t.Run(name, func(t *testing.T) {
			dir := labelledDir
			if heldout[name] {
				dir = heldoutDir
			}
			_, e, result := replayScenario(t, dir, name)
			v := judge(e, result)
			t.Logf("%s: %d messages; %s", outcome(v), v.messages,
				describeIncidents(v.incidents))
			if *only == "" {
				return
			}
			for i, m := range result.Messages {
				t.Logf("--- %s %s (%s)\n%s", result.Times[i].Format(
					"15:04:05"), result.Decisions[i].Reason,
					result.Decisions[i].Incident.Root, notification.Text(m))
			}
			for _, p := range result.Incidents {
				t.Logf("incident %s %s %s members: %s", p.ID, p.Root,
					p.Tier, describeMembers(p))
			}
		})
	}
}

// describeMembers lists an incident's members as "reason@since", so a
// scenario's grouping can be read while it is written.
func describeMembers(p incident.Incident) string {
	var parts []string
	for key, f := range p.Members {
		parts = append(parts, key.Entity.String()+" "+f.Reason+"@"+
			f.Since.Format("15:04:05"))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func describeIncidents(incidents []reported) string {
	parts := make([]string, 0, len(incidents))
	for _, p := range incidents {
		part := p.Root + " " + p.Tier.String()
		if p.Cause != nil {
			part += " cause=" + p.Cause.Rule
		}
		parts = append(parts, part)
	}
	return "incidents: [" + strings.Join(parts, "; ") + "]"
}
