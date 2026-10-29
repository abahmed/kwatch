package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// restartedContainer is a running container of a pod started podAge
// before t0, with restarts in all and the last one a minute ago.
func restartedContainer(
	podAge time.Duration, restarts float64,
) []detection.Finding {
	m := newTestModel()
	pod := newID(kube.KindPod, "shop", "api-1")
	container := newID(kube.KindContainer, "shop", "api-1/app")
	put(m, pod, t0.Add(-podAge), map[string]inventory.Value{
		kube.AttrStartTime: inventory.Time(t0.Add(-podAge)),
	})
	put(m, container, t0.Add(-time.Minute), map[string]inventory.Value{
		kube.AttrState:        inventory.Text("running"),
		kube.AttrRestarts:     inventory.Number(restarts),
		kube.AttrLastFinished: inventory.Time(t0.Add(-time.Minute)),
	})
	link(m, container, inventory.PartOf, pod)
	return evaluate(Container{}, m, t0, container, nil).Findings
}

// A long-lived pod whose few restarts are spread over months is not in a
// restart loop when it restarts once more; a young pod restarting fast,
// or one that keeps crashing every few hours for days, is.
func TestContainerRestartLoopNeedsFrequentRestarts(t *testing.T) {
	tests := []struct {
		name     string
		podAge   time.Duration
		restarts float64
		want     bool
	}{
		{"five restarts in three months", 90 * 24 * time.Hour, 5, false},
		{"five restarts in an hour", time.Hour, 5, true},
		{"every two hours for ten days", 10 * 24 * time.Hour, 120, true},
	}
	for _, tt := range tests {
		got := hasReason(restartedContainer(tt.podAge, tt.restarts),
			"HighRestartCount")
		assert.Equal(t, tt.want, got, tt.name)
	}
}
