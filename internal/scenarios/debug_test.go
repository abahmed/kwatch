package scenarios

import (
	"flag"
	"strings"
	"testing"

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
	for _, s := range library() {
		name := s.expect.Name
		if *only != "" && name != *only {
			continue
		}
		t.Run(name, func(t *testing.T) {
			log, e := loadScenario(t, name)
			result := replayLog(t, log, e.options(log.Start))
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
		})
	}
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
