package compose_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

// ExampleWriter_Write turns the announcement of one incident, a
// Deployment whose pods crash on a missing setting, into the note people
// read. The writer reads the incident only; it does no I/O.
func ExampleWriter_Write() {
	start := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	deploy := inventory.CoreID(kube.KindDeployment, "shop", "payments")
	container := inventory.CoreID(kube.KindContainer, "shop",
		"payments-7d9f/app")
	crash := detection.Finding{
		Entity: container, Reason: reasons.CrashLoopBackOff,
		Severity: detection.Critical, Since: start,
		Summary: "Container is crash looping",
		Evidence: []detection.Evidence{{
			Label: "error", Value: "panic: STRIPE_KEY is not set"}},
	}
	decision := incident.Decision{
		Action: incident.Announce, Reason: "settled",
		Incident: incident.Incident{
			ID: "inc-1", Root: deploy, Tier: incident.Page,
			State: incident.Open, Opened: start, Revision: 1,
			Members: map[detection.Key]detection.Finding{
				crash.Key(): crash},
			Impact: []inventory.EntityID{deploy},
		},
	}
	message := compose.Writer{}.Write(decision, start.Add(3*time.Minute))
	// Break the note at sentence and clause ends to keep lines short.
	lines := strings.NewReplacer(". ", ".\n", ", ", ",\n")
	fmt.Println(lines.Replace(message.Note))
	// Output:
	// 🔴 payments in shop is crash looping,
	// and I couldn't find an outside cause.
	// It fails with "panic: STRIPE_KEY is not set".
	// To read the output of the last crash,
	// run kubectl logs payments-7d9f -c app -n shop --previous
}
