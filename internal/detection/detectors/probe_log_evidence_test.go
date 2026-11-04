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

const appHealthLine = "health check failed: database not ready"

// detectReadiness runs the container detector on a running, unready
// container whose readiness probe failed with message and whose current
// log names line.
func detectReadiness(
	t *testing.T, message, line string,
) []detection.Finding {
	t.Helper()
	model := newTestModel()
	id := kube.ContainerID("default", "app", "main")
	attrs := map[string]inventory.Value{
		kube.AttrState:     inventory.Text("running"),
		kube.AttrReady:     inventory.Bool(false),
		kube.AttrStartedAt: inventory.Time(podNodeNow.Add(-time.Hour)),
	}
	if line != "" {
		attrs[kube.AttrProbeLogLine] = inventory.Text(line)
	}
	observeEntity(model, id, podNodeNow.Add(-10*time.Minute), attrs)
	noteEntity(model, id, "Unhealthy", message, 5,
		podNodeNow.Add(-time.Minute))
	return Container{}.Detect(testDetectorContext(model, podNodeNow),
		entityOf(model, id))
}

func TestProbeLogEvidenceQuotesTheAppWhenItAnswersAnError(t *testing.T) {
	found := detectReadiness(t, "Readiness probe failed: HTTP probe "+
		"failed with statuscode: 500", appHealthLine)

	require.Len(t, found, 1)
	assert.Equal(t, reasons.ReadinessProbeFailed, found[0].Reason)
	assert.Equal(t, appHealthLine,
		evidenceOf(found[0], detection.EvidenceProbeLog))
	assert.NotContains(t, found[0].Summary, appHealthLine,
		"the summary is the same in every edition")
}

func TestProbeLogEvidenceIsLeftOutWhenNothingAnswered(t *testing.T) {
	found := detectReadiness(t, "Readiness probe failed: Get "+
		"\"http://10.0.0.9:8080/ready\": dial tcp 10.0.0.9:8080: "+
		"connect: connection refused", appHealthLine)

	require.Len(t, found, 1)
	assert.Empty(t, evidenceOf(found[0], detection.EvidenceProbeLog))
}

func TestProbeLogEvidenceNeedsALine(t *testing.T) {
	found := detectReadiness(t, "Readiness probe failed: HTTP probe "+
		"failed with statuscode: 503", "")

	require.Len(t, found, 1)
	assert.Empty(t, evidenceOf(found[0], detection.EvidenceProbeLog))
}
