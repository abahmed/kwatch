package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// nodeOOMKill is an OOM-killed container the detector says the node
// killed: 180Mi used of a 512Mi limit, on a node whose biggest user is
// an importer without a limit.
func nodeOOMKill() incident.Incident {
	p := oomKill(
		detection.Evidence{Label: detection.EvidenceKilledByNode,
			Value: "ip-10-0-1-2"},
		detection.Evidence{Label: detection.EvidenceMemoryUsed,
			Value: "180Mi"},
		detection.Evidence{Label: detection.EvidenceMemoryLimit,
			Value: "512Mi"},
		detection.Evidence{Label: detection.EvidenceNodeMemoryUser,
			Value: "batch/importer 3.1Gi (no limit)"},
		detection.Evidence{Label: detection.EvidenceNodeMemoryUser,
			Value: "shop/search 900Mi"})
	for key, m := range p.Members {
		m.Summary = "Container was OOM-killed by the node and not by " +
			"its own limit"
		p.Members[key] = m
	}
	return p
}

func TestWriteNodeOOMNamesTheNodeAndItsBiggestUsers(t *testing.T) {
	got := Writer{}.Write(announce(nodeOOMKill()), writerNow).Note

	assert.Contains(t, got, "was OOM-killed by the node and not by its "+
		"own limit")
	assert.Contains(t, got, "It used 180Mi of its 512Mi limit.")
	assert.Contains(t, got, "Node ip-10-0-1-2 ran out of memory; biggest "+
		"users: batch/importer 3.1Gi (no limit) and shop/search 900Mi.")
	assert.NotContains(t, got, "killed at its")
}

func TestWriteNodeOOMDoesNotAdviseRaisingTheLimit(t *testing.T) {
	steps := Writer{}.Write(announce(nodeOOMKill()), writerNow).Steps

	require.NotEmpty(t, steps)
	for _, step := range steps {
		assert.NotContains(t, step.Text, "Raise the memory limit")
	}
	assert.Equal(t, "kubectl describe node ip-10-0-1-2", steps[0].Command)
}

func TestWriteOwnLimitOOMKeepsItsWording(t *testing.T) {
	got := note(t,
		detection.Evidence{Label: detection.EvidenceMemoryLimit,
			Value: "512Mi"},
		detection.Evidence{Label: detection.EvidenceMemoryPeak,
			Value: "512Mi"})

	assert.NotContains(t, got, "by the node")
	assert.NotContains(t, got, "ran out of memory")
}

// throttledProbe is a container whose liveness probe fails while it is
// throttled on CPU.
func throttledProbe(message string) incident.Incident {
	id := inventory.CoreID(kube.KindContainer, "shop", "web-1/app")
	return incident.Incident{
		ID: "probe-1", Root: id, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(detection.Finding{Entity: id,
			Reason: reasons.LivenessProbeFailed, Severity: detection.Warning,
			Since:   writerNow.Add(-time.Hour),
			Summary: "Container keeps failing its liveness probe",
			Evidence: []detection.Evidence{
				{Label: "probe", Value: message},
				{Label: detection.EvidenceCPUThrottled, Value: "72%"},
				{Label: detection.EvidenceCPULimit, Value: "200m"},
			}}),
	}
}

func TestWriteProbeTimeoutSaysTheContainerWasThrottled(t *testing.T) {
	got := Writer{}.Write(announce(throttledProbe(
		"Liveness probe failed: Get \"http://10.0.0.1/healthz\": "+
			"context deadline exceeded")), writerNow).Note

	assert.Contains(t, got, "Liveness timed out while the container was "+
		"CPU-throttled 72% of the time (limit 200m).")
}

func TestWriteProbeFailureSaysTheContainerWasThrottled(t *testing.T) {
	got := Writer{}.Write(announce(throttledProbe(
		"Liveness probe failed: HTTP probe failed with statuscode: 503")),
		writerNow).Note

	assert.Contains(t, got, "Liveness failed while the container was "+
		"CPU-throttled 72% of the time (limit 200m).")
}
