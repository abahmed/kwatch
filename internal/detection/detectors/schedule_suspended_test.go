package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A suspended CronJob or Job was paused on purpose: the finding is
// advisory, so it is context in the incident of a real failure and
// never opens one ("is failing: suspended").
func TestScheduleSuspendedIsAdvisory(t *testing.T) {
	for _, kind := range []inventory.Kind{kube.KindCronJob, kube.KindJob} {
		t.Run(string(kind), func(t *testing.T) {
			m := newTestModel()
			id := newID(kind, "shop", "rollup")
			put(m, id, t0, map[string]inventory.Value{
				kube.AttrSuspended: inventory.Bool(true),
			})
			got := evaluate(Schedule{}, m, t0, id, nil).Findings
			require.Len(t, got, 1)
			assert.True(t, got[0].Advisory)
		})
	}
}
