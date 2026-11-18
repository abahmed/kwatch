package investigate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestMemoryContextSectionsListTheKillContext(t *testing.T) {
	kill := detection.Finding{
		Entity: kube.ContainerID("shop", "api-1", "app"),
		Reason: reasons.OOMKilled, Evidence: []detection.Evidence{
			{Label: detection.EvidenceOOMKind,
				Value: "its own memory limit"},
			{Label: detection.EvidenceMemoryWindow, Value: "5h"},
			{Label: detection.EvidenceNodeMemoryUser,
				Value: "batch/importer 3.1Gi (request 1Gi)"},
		}}
	p := incident.Incident{Members: map[detection.Key]detection.Finding{
		kill.Key(): kill}}
	got := memoryContextSections(p)
	require.Len(t, got, 1)
	assert.Equal(t, []incident.SectionFact{
		{Label: "Limit reached", Value: "its own memory limit"},
		{Label: "History kept", Value: "5h"},
		{Label: "Node memory user",
			Value: "batch/importer 3.1Gi (request 1Gi)"},
	}, got[0].Facts)
}

func TestMemoryContextSectionsSkipOtherFailures(t *testing.T) {
	crash := detection.Finding{
		Entity: kube.ContainerID("shop", "api-1", "app"),
		Reason: reasons.CrashLoopBackOff}
	p := incident.Incident{Members: map[detection.Key]detection.Finding{
		crash.Key(): crash}}
	assert.Empty(t, memoryContextSections(p))
}
