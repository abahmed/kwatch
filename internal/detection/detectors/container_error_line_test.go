package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// crashEvidence returns the "error" evidence of a crash-looping
// container with the given termination message and log error line.
func crashEvidence(message, logLine string) []string {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	attrs := map[string]inventory.Value{
		kube.AttrState:        inventory.Text("waiting"),
		kube.AttrStateReason:  inventory.Text(reasons.CrashLoopBackOff),
		kube.AttrLastExitCode: inventory.Number(143),
	}
	if message != "" {
		attrs[kube.AttrLastMessage] = inventory.Text(message)
	}
	if logLine != "" {
		attrs[kube.AttrLastErrorLine] = inventory.Text(logLine)
	}
	pod := inventory.EntityID{Kind: kube.KindPod, Namespace: "ns",
		Name: "p"}
	container := buildContainer(pod, "app", now, attrs)
	var out []string
	for _, f := range (Container{}).Detect(testDetectorContext(nil, now),
		container) {
		for _, e := range f.Evidence {
			if e.Label == "error" {
				out = append(out, e.Value)
			}
		}
	}
	return out
}

func TestContainerErrorEvidenceFallsBackToLogLine(t *testing.T) {
	assert.Equal(t, []string{"redis down"},
		crashEvidence("", "redis down"))
}

func TestContainerErrorEvidencePrefersTerminationMessage(t *testing.T) {
	assert.Equal(t, []string{"boom"}, crashEvidence("boom", "redis down"))
}

func TestContainerErrorEvidenceAbsentWithoutEither(t *testing.T) {
	assert.Empty(t, crashEvidence("", ""))
}
