package investigate

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const appLogLine = "health check failed: database not ready"

// probeLogSources is a running container whose readiness probe gets
// result, with attrs on top, and a current log of lines whose reads are
// counted.
func probeLogSources(
	t *testing.T, result string, attrs map[string]inventory.Value,
	lines []string,
) (Sources, incident.Incident, *int) {
	t.Helper()
	container := kube.ContainerID("shop", "api-1", "app")
	all := map[string]inventory.Value{kube.AttrState: text("running")}
	for k, v := range attrs {
		all[k] = v
	}
	model := testModel(t, observed(container, all))
	reads := 0
	read := func(context.Context, inventory.EntityID) []string {
		reads++
		return lines
	}
	p := incidentOf(container, finding(container, "Readiness",
		detection.Evidence{Label: detection.EvidenceProbeResult,
			Value: result}))
	return Sources{Model: model, CurrentLogs: read}, p, &reads
}

func TestInvestigateProbeLogReadsTheCurrentRun(t *testing.T) {
	sources, p, reads := probeLogSources(t, "HTTP 500", nil,
		[]string{"info: start", appLogLine, "info: tick"})

	plan, r := planAndRun(t, sources, p)

	if plan.Kind != incident.CheckProbeLog {
		t.Fatalf("kind = %q, want %s", plan.Kind, incident.CheckProbeLog)
	}
	got := evidenceOf(r, incident.FactProbeLog)
	if len(got) != 1 || got[0].Text != appLogLine || *reads != 1 {
		t.Fatalf("evidence = %+v after %d reads", got, *reads)
	}
}

func TestInvestigateProbeLogUsesTheLineTheRoundRead(t *testing.T) {
	sources, p, reads := probeLogSources(t, "HTTP 503",
		map[string]inventory.Value{kube.AttrProbeLogLine: text(appLogLine)},
		nil)

	_, r := planAndRun(t, sources, p)

	got := evidenceOf(r, incident.FactProbeLog)
	if len(got) != 1 || got[0].Text != appLogLine || *reads != 0 {
		t.Fatalf("evidence = %+v after %d reads", got, *reads)
	}
}

func TestInvestigateProbeLogSkipsProbesNobodyAnswered(t *testing.T) {
	sources, p, reads := probeLogSources(t, "connection refused", nil,
		[]string{appLogLine})

	plan, ok := NewInvestigator(sources).Plan(p)
	if ok && plan.Kind == incident.CheckProbeLog {
		t.Fatal("a refused probe does not lead a probe log read")
	}
	if ok {
		r := plan.Run(context.Background())
		if got := evidenceOf(r, incident.FactProbeLog); len(got) > 0 {
			t.Fatalf("evidence = %+v", got)
		}
	}
	if *reads != 0 {
		t.Fatalf("%d reads of a container nothing answered for", *reads)
	}
}

func TestInvestigateProbeLogLeavesAStoppedContainer(t *testing.T) {
	sources, p, reads := probeLogSources(t, "HTTP 500",
		map[string]inventory.Value{kube.AttrState: text("waiting")},
		[]string{appLogLine})

	if plan, ok := NewInvestigator(sources).Plan(p); ok &&
		plan.Kind == incident.CheckProbeLog {
		t.Fatal("a container that is not running is not read")
	}
	if *reads != 0 {
		t.Fatalf("%d reads", *reads)
	}
}

func TestInvestigateProbeLogWithLogsOffReadsNothing(t *testing.T) {
	sources, p, reads := probeLogSources(t, "HTTP 500", nil,
		[]string{appLogLine})
	sources.LogsOff = true

	_, r := planAndRun(t, sources, p)

	if got := evidenceOf(r, incident.FactProbeLog); len(got) != 0 ||
		*reads != 0 {
		t.Fatalf("evidence = %+v after %d reads", got, *reads)
	}
}
