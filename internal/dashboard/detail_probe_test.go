package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// probeSettings are a container's settings of one probe.
func probeSettings(
	probe, check string, period, timeout, failures float64,
) map[string]inventory.Value {
	return map[string]inventory.Value{
		kube.ProbeAttr(probe, "check"):    inventory.Text(check),
		kube.ProbeAttr(probe, "delay"):    inventory.Number(0),
		kube.ProbeAttr(probe, "period"):   inventory.Number(period),
		kube.ProbeAttr(probe, "timeout"):  inventory.Number(timeout),
		kube.ProbeAttr(probe, "failures"): inventory.Number(failures),
	}
}

func TestDetailProbesReadSettingsAndTheLatestFailure(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	api := tm.container(pod, "api",
		probeSettings("liveness", "http /healthz :8080", 10, 1, 3))
	tm.note(api, "Unhealthy", "Liveness probe failed: Get "+
		"\"http://10.0.0.1:8080/healthz\": context deadline exceeded", 12,
		time.Minute)
	tm.note(api, "Unhealthy2", "Readiness probe failed: other", 3,
		30*time.Second)

	d := detailOf(t, contextServer(tm.m, false,
		failingWith("p", root, reasons.LivenessProbeFailed, api)), "p")
	if len(d.Probes) != 1 {
		t.Fatalf("probes = %+v", d.Probes)
	}
	p := d.Probes[0]
	if p.Pod != "api-1" || p.Container != "api" || p.Probe != "liveness" ||
		p.Check != "http /healthz :8080" || p.Period != 10 ||
		p.Timeout != 1 || p.Failures != 3 || p.Count != 12 ||
		!strings.HasPrefix(p.LastMessage, "Liveness probe failed") ||
		strings.Contains(p.LastMessage, "10.0.0.1") ||
		!p.LastAt.Equal(testEpoch.Add(-time.Minute)) {
		t.Errorf("probe = %+v", p)
	}
}

func TestDetailProbesNameOnlyTheExecProgram(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	attrs := probeSettings("readiness", "exec", 5, 2, 1)
	attrs[kube.ProbeAttr("readiness", "command")] =
		inventory.Text("grpc_health_probe")
	tm.container(pod, "api", attrs)
	tm.container(pod, "sidecar", nil)

	d := detailOf(t, contextServer(tm.m, false,
		failingWith("p", root, reasons.ReadinessProbeFailed, pod)), "p")
	if len(d.Probes) != 1 || d.Probes[0].Check != "exec grpc_health_probe" ||
		d.Probes[0].Container != "api" || d.Probes[0].LastMessage != "" {
		t.Fatalf("probes = %+v", d.Probes)
	}
}

func TestDetailProbesAreBoundedAndAbsentForOtherFailures(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	var ids []inventory.EntityID
	for i := range maxProbes + 2 {
		ids = append(ids, tm.container(pod, fmt.Sprintf("c%d", i),
			probeSettings("startup", "tcp :5432", 10, 1, 30)))
	}
	d := detailOf(t, contextServer(tm.m, false,
		failingWith("p", root, reasons.StartupProbeFailed, ids...)), "p")
	if len(d.Probes) != maxProbes || d.Probes[0].Container != "c0" ||
		d.Truncated == nil || d.Truncated.Probes != 2 {
		t.Fatalf("probes = %+v, truncated %+v", d.Probes, d.Truncated)
	}
	d = detailOf(t, contextServer(tm.m, false,
		failingWith("q", root, "CrashLoopBackOff", ids...)), "q")
	if d.Probes != nil {
		t.Fatalf("a crash loop has probes: %+v", d.Probes)
	}
}
