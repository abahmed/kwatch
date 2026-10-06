package coverage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestCoverageWatchNeedsFifteenMinutesAndRestartsOrNoneReady(t *testing.T) {
	w := &Watch{}
	model := inventory.NewModel(inventory.Options{})
	id := inventory.CoreID(kube.KindDeployment, "shop", "api")
	set := func(ready float64) {
		_, err := model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: "test", Entity: id,
			Attributes: map[string]inventory.Value{
				kube.AttrReplicas:      inventory.Number(3),
				kube.AttrReadyReplicas: inventory.Number(ready),
			}})
		require.NoError(t, err)
	}
	start := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)

	set(1)
	assert.Empty(t, w.FailingWorkloads(model, start), "just fell short")
	assert.Empty(t, w.FailingWorkloads(model, start.Add(10*time.Minute)))
	assert.Empty(t, w.FailingWorkloads(model, start.Add(15*time.Minute)),
		"short for 15 minutes but not restarting and some are ready")

	set(0)
	assert.Equal(t, []inventory.EntityID{id},
		w.FailingWorkloads(model, start.Add(20*time.Minute)),
		"nothing ready")

	set(3)
	assert.Empty(t, w.FailingWorkloads(model, start.Add(25*time.Minute)))
	set(0)
	assert.Empty(t, w.FailingWorkloads(model, start.Add(26*time.Minute)),
		"the wait starts over once it was healthy")
}

// The handed-back record is bounded by the cluster: a workload that left
// the model leaves it too.
func TestCoverageWatchForgetsHandedWorkloadsThatAreGone(t *testing.T) {
	w := &Watch{}
	model := inventory.NewModel(inventory.Options{})
	gone := inventory.CoreID(kube.KindDeployment, "shop", "gone")
	w.FailingWorkloads(model, time.Time{})
	w.HandBack(gone, time.Now())

	w.FailingWorkloads(model, time.Time{}.Add(time.Hour))

	assert.Empty(t, w.handed)
}
