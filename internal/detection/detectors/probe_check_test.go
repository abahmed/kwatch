package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A failing probe carries what it asks and its timeout, from the spec,
// so a message can name the check: "/readyz on port 8081".
func TestProbeFindingCarriesItsCheckAndTimeout(t *testing.T) {
	model := newTestModel()
	id := kube.ContainerID("default", "app", "main")
	observeEntity(model, id, podNodeNow.Add(-10*time.Minute),
		map[string]inventory.Value{
			kube.AttrState:     inventory.Text("running"),
			kube.AttrReady:     inventory.Bool(false),
			kube.AttrStartedAt: inventory.Time(podNodeNow.Add(-time.Hour)),
			kube.ProbeAttr("readiness", "check"): inventory.Text(
				"http /readyz :8081"),
			kube.ProbeAttr("readiness", "timeout"): inventory.Number(1),
		})
	noteEntity(model, id, "Unhealthy", "Readiness probe failed: HTTP "+
		"probe failed with statuscode: 500", 5, podNodeNow.Add(-time.Minute))

	found := Container{}.Detect(testDetectorContext(model, podNodeNow),
		entityOf(model, id))
	require.Len(t, found, 1)
	ev := map[string]string{}
	for _, e := range found[0].Evidence {
		ev[e.Label] = e.Value
	}
	assert.Equal(t, "http /readyz :8081", ev[detection.EvidenceProbeCheck])
	assert.Equal(t, "1", ev[detection.EvidenceProbeTimeout])
	assert.Equal(t, "HTTP 500", ev[detection.EvidenceProbeResult])
}
