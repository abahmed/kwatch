package release

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A Deployment that is slow but has not missed its progress deadline is
// still rolling, and ends healthy, not failed.
func TestSlowDeploymentIsNotStalled(t *testing.T) {
	w := newWorld(kube.KindDeployment, "api")
	w.status(100, 100, 40, "")
	w.change(t0, "", image("api:1", "api:2"))
	var b Book
	w.look(&b, t0.Add(StallAfter+time.Minute))
	assert.Equal(t, Rolling, b.Records[0].Verdict)
	w.status(100, 100, 100, "")
	w.look(&b, t0.Add(20*time.Minute))
	w.look(&b, t0.Add(20*time.Minute+WatchWindow+time.Minute))
	assert.Equal(t, Healthy, b.Records[0].Verdict)
}

// A StatefulSet that keeps moving is not stalled however long it takes;
// one that stops moving for StallAfter is.
func TestStatefulSetStallsOnlyWhenItStopsMoving(t *testing.T) {
	w := newWorld(kube.KindStatefulSet, "db")
	w.status(10, 10, 2, "")
	w.change(t0, "", image("db:1", "db:2"))
	var b Book
	w.look(&b, t0.Add(10*time.Minute))
	w.status(10, 10, 6, "")
	w.look(&b, t0.Add(16*time.Minute))
	assert.Equal(t, Rolling, b.Records[0].Verdict)
	w.look(&b, t0.Add(16*time.Minute+StallAfter))
	assert.Equal(t, Failed, b.Records[0].Verdict)
}

// A rollout that waits for people on purpose never stalls.
func TestOnDeleteAndPartitionNeverStall(t *testing.T) {
	for name, extra := range map[string]map[string]inventory.Value{
		"on delete": {kube.AttrUpdateStrategy: inventory.Text("OnDelete")},
		"partition": {kube.AttrPartition: inventory.Number(2)},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(kube.KindStatefulSet, "db")
			w.statusWith(3, 3, 1, "", extra)
			w.change(t0, "", image("db:1", "db:2"))
			var b Book
			w.look(&b, t0.Add(time.Minute))
			w.look(&b, t0.Add(time.Hour))
			assert.Equal(t, Rolling, b.Records[0].Verdict)
		})
	}
}
