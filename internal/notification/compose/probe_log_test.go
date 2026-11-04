package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const probeLogLine = "health check failed: database not ready"

// unreadyApp is a running container whose readiness probe gets result,
// with the line its current log names, if any.
func unreadyApp(result, line string) incident.Incident {
	evidence := []detection.Evidence{
		{Label: "probe", Value: "Readiness probe failed: HTTP probe " +
			"failed with statuscode: 500"},
		{Label: detection.EvidenceProbeResult, Value: result},
	}
	if line != "" {
		evidence = append(evidence, detection.Evidence{
			Label: detection.EvidenceProbeLog, Value: line})
	}
	container := inventory.CoreID(kube.KindContainer, "shop",
		"api-7d9f/app")
	return incident.Incident{
		ID: "probe-1", Root: container, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(detection.Finding{
			Entity: container, Reason: reasons.ReadinessProbeFailed,
			Severity: detection.Warning, Since: writerNow.Add(-time.Hour),
			Summary: "Container keeps failing its own readiness check " +
				"(HTTP 500) while it runs",
			Evidence: evidence,
		}),
	}
}

func TestWriteProbeLogQuotesTheAppNextToTheProbeResult(t *testing.T) {
	got := Writer{}.Write(announce(unreadyApp("HTTP 500", probeLogLine)),
		writerNow).Note

	assert.Contains(t, got, "Its readiness check answers HTTP 500 (from "+
		"its log: \""+probeLogLine+"\").")
}

func TestWriteProbeLogNeedsAnAnsweredProbeAndALine(t *testing.T) {
	for _, p := range []incident.Incident{
		unreadyApp("HTTP 500", ""),
		unreadyApp("connection refused", probeLogLine),
	} {
		got := Writer{}.Write(announce(p), writerNow).Note
		assert.NotContains(t, got, "from its log")
	}
}

func TestWriteProbeLogIsLeftOutOfCommunity(t *testing.T) {
	community := Writer{}.ForEdition(feature.CommunityEntitlements(), true)
	got := community.Write(
		announce(unreadyApp("HTTP 500", probeLogLine)), writerNow).Note

	assert.NotContains(t, got, probeLogLine)
	assert.NotContains(t, got, "from its log")
}
