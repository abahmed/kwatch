package explain

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

var addressHolderRowCases = []rowCase{
	{row: RuleFinalizerHoldsAddresses,
		want: "finalizer/jobs/" + widgetFinalizer,
		build: func(f *fixture) inventory.EntityID {
			nodes := f.nodes("zone-a", "n1")
			for _, pod := range f.workload("jobs", "runner", 2, nodes[0]) {
				f.findings[pod] = append(f.findings[pod], detection.Finding{
					Entity: pod, Reason: "StuckDeleting",
					Mode: detection.ModeStuckDeleting, Health: degradedH,
					Since:   t0.Add(-3 * time.Hour),
					Summary: "Deletion is blocked",
					Evidence: []detection.Evidence{{Label: "finalizers",
						Value: strings.Join([]string{widgetFinalizer},
							", ")}},
				})
			}
			web := f.workload("shop", "web", 1, nodes[0])
			f.fail(web[0], detection.ModeNetworkIPExhausted, failingH, 10,
				"failed to assign an IP address to container")
			return web[0]
		}},
}
