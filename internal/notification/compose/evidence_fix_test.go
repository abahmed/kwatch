package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// oomDecision is a Deployment whose container was killed at its limit
// after peaking at peak.
func oomDecision(peak string, report []incident.Section) incident.Decision {
	root := inventory.CoreID(kube.KindDeployment, "shop", "api")
	id := inventory.CoreID(kube.KindContainer, "shop", "api-7f-x/app")
	f := detection.Finding{Entity: id, Reason: reasons.OOMKilled,
		Mode: "OOMKilled", Severity: detection.Critical,
		Since: evidenceAt.Add(-5 * time.Minute),
		Evidence: []detection.Evidence{
			{Label: detection.EvidenceMemoryLimit, Value: "256Mi"},
			{Label: detection.EvidenceMemoryPeak, Value: peak},
			{Label: "last termination", Value: "OOMKilled"}}}
	p := incident.Incident{ID: "inc-2", Root: root, State: incident.Open,
		Opened: f.Since, Revision: 1,
		Members: map[detection.Key]detection.Finding{f.Key(): f},
		Cause: &rootcause.CauseRecord{Root: root, Score: rootcause.High,
			Summary: "api runs out of memory"}}
	return incident.Decision{Action: incident.Announce, Incident: p,
		Facts: incident.Facts{Report: report}}
}

func TestWriteProSuggestsALimitFromThePeak(t *testing.T) {
	m := NewWriter("", nil).Write(oomDecision("255Mi", gatheredReport),
		evidenceAt)
	want := "kubectl set resources deployment/api -n shop -c app " +
		"--limits=memory=384Mi"
	var found bool
	for _, s := range m.Steps {
		found = found || (s.Command == want && s.Mutating)
	}
	if !found || !strings.Contains(m.Note, want) {
		t.Fatalf("steps %+v\nnote %s", m.Steps, m.Note)
	}
	if m.Evidence == nil || len(m.Evidence.Fix) != 1 ||
		m.Evidence.Fix[0].Command != want {
		t.Fatalf("evidence = %+v", m.Evidence)
	}
}

func TestWriteProSizesFromTheLimitWhenThePeakIsLower(t *testing.T) {
	m := NewWriter("", nil).Write(oomDecision("100Mi", gatheredReport),
		evidenceAt)
	if !strings.Contains(commandsOf(m), "--limits=memory=384Mi") {
		t.Fatalf("steps = %s", commandsOf(m))
	}
}

func TestWriteCommunityGetsNoComputedFix(t *testing.T) {
	w := NewWriter("", nil).ForEdition(feature.OrCommunity(nil), false)
	with := w.Write(oomDecision("255Mi", gatheredReport), evidenceAt)
	without := w.Write(oomDecision("255Mi", nil), evidenceAt)
	if strings.Contains(commandsOf(with), "set resources") ||
		with.Evidence != nil || with.Note != without.Note {
		t.Fatalf("Community changed: %s", commandsOf(with))
	}
}

func TestMebibytes(t *testing.T) {
	for in, want := range map[string]float64{"610Mi": 610, "1.5Gi": 1536,
		"512Ki": 0.5, "12": 0, "": 0, "xMi": 0} {
		if got := mebibytes(in); got != want {
			t.Errorf("mebibytes(%q) = %v, want %v", in, got, want)
		}
	}
}
