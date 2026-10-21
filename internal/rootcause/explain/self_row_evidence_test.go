package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// probeRefusedText is what the kubelet reports for a probe that cannot
// connect: it does not name the workload.
const probeRefusedText = "dial tcp 10.0.0.5:8081: connect: connection " +
	"refused"

// failWithOutput gives id a finding whose evidence is the program's own
// last words (the "error" evidence) next to the kubelet's message.
func (f *fixture) failWithOutput(
	id inventory.EntityID, mode detection.Mode, output string,
) {
	f.findings[id] = append(f.findings[id], detection.Finding{
		Entity: id, Reason: string(mode), Mode: mode,
		Health: degradedH, Since: t0.Add(2 * time.Minute), Summary: string(mode),
		Evidence: []detection.Evidence{
			{Label: "message", Value: probeRefusedText},
			{Label: detection.EvidenceError, Value: output}},
	})
}

// probeMismatchCase probes an undeclared port of api; its pods say
// output as their own last words. It returns the workload.
func probeMismatchCase(f *fixture, output string) inventory.EntityID {
	pods := f.workload("shop", "api", 2)
	for _, pod := range pods {
		f.observe(containerOf(pod), map[string]inventory.Value{
			kube.AttrContainerPorts: inventory.Text("8080"),
			kube.AttrProbePorts:     inventory.Text("8081"),
		})
		f.failWithOutput(containerOf(pod), "Probe.Readiness", output)
	}
	return inventory.CoreID(kube.KindDeployment, "shop", "api")
}

// TestCoverageGivesAWorkloadNoCreditForItsOwnPods: the pods of a
// workload blamed for its own setting are the failures, not support
// for the setting.
func TestCoverageGivesAWorkloadNoCreditForItsOwnPods(t *testing.T) {
	f := newFixture(t)
	workload := probeMismatchCase(f, "ready")

	if o := scoreOf(t, f, workload, scoreCoverage); o.weight > 0 {
		t.Fatalf("own pods were counted as support: %+v", o)
	}
}

// TestCoverageStillCountsAgainstASelfRowWhenFewPodsFail: one pod in four
// failing speaks against a setting that would break every pod.
func TestCoverageStillCountsAgainstASelfRowWhenFewPodsFail(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("shop", "api", 4)
	f.observe(containerOf(pods[0]), map[string]inventory.Value{
		kube.AttrContainerPorts: inventory.Text("8080"),
		kube.AttrProbePorts:     inventory.Text("8081"),
	})
	f.failWithOutput(containerOf(pods[0]), "Probe.Readiness", "ready")
	workload := inventory.CoreID(kube.KindDeployment, "shop", "api")

	if o := scoreOf(t, f, workload, scoreCoverage); o.weight >= 0 {
		t.Fatalf("one failing pod in four should lower it: %+v", o)
	}
}

// TestSpecificityIgnoresTheProgramsOwnOutputForSelfRows: an app names
// itself in its own logs ("orders: db down"); that is no evidence that
// its probe port is wrong.
func TestSpecificityIgnoresTheProgramsOwnOutputForSelfRows(t *testing.T) {
	f := newFixture(t)
	workload := probeMismatchCase(f, "api: cannot reach the database")

	if o := scoreOf(t, f, workload, scoreSpecificity); o.said() {
		t.Fatalf("own output named the workload: %+v", o)
	}
}

// TestSpecificityStillCountsTheKubeletNamingTheWorkload: what the
// cluster reports about the workload, outside the program's output,
// still counts.
func TestSpecificityStillCountsTheKubeletNamingTheWorkload(t *testing.T) {
	f := newFixture(t)
	workload := probeMismatchCase(f, "ready")
	for _, pod := range f.model.Entities(kube.KindPod) {
		f.note(pod, "Readiness probe of api failed: connection refused")
	}

	if o := scoreOf(t, f, workload, scoreSpecificity); !o.said() {
		t.Fatalf("the kubelet naming the workload was not counted")
	}
}
