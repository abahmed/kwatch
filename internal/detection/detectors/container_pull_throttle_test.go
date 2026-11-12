package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A pull the kubelet held back under its own pull rate limit is only a
// warning: the registry was never asked and the kubelet retries.
func TestContainerPullQPSExceededIsAWarning(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	container := buildContainer(
		inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: "pod-1"},
		"app", now.Add(-time.Minute),
		map[string]inventory.Value{
			kube.AttrState:       inventory.Text("waiting"),
			kube.AttrStateReason: inventory.Text(reasons.ErrImagePull),
			kube.AttrMessage:     inventory.Text("pull QPS exceeded"),
		},
	)
	got := Container{}.Detect(testDetectorContext(nil, now), container)

	require.Len(t, got, 1)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Contains(t, got[0].Summary, "limits how fast it pulls images")
}
