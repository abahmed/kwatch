package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// probePortCase probes an undeclared port; both pods fail in mode with
// text. It returns a failing container.
func probePortCase(
	f *fixture, mode detection.Mode, health detection.Health, text string,
) inventory.EntityID {
	pods := f.workload("shop", "api", 2)
	for _, pod := range pods {
		f.observe(containerOf(pod), map[string]inventory.Value{
			kube.AttrContainerPorts: inventory.Text("8080"),
			kube.AttrProbePorts:     inventory.Text("8081"),
		})
		f.fail(containerOf(pod), mode, health, 2, text)
	}
	return containerOf(pods[0])
}

// TestProbePortMismatchNeedsAProbeThatCannotConnect: the probe is only
// blamed when the failure says nothing listens. A crash with any other
// words is the program's own, whatever port its probe uses.
func TestProbePortMismatchNeedsAProbeThatCannotConnect(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mode   detection.Mode
		health detection.Health
		text   string
		want   bool
	}{
		{"readiness cannot connect", "Probe.Readiness", degradedH,
			"dial tcp 10.0.0.5:8081: connect: connection refused", true},
		{"liveness cannot connect", "CrashLoop.Liveness", failingH,
			"Liveness probe failed: connection refused", true},
		{"crash on a database error", "CrashLoop", failingH,
			"fatal: password authentication failed for user app", false},
		{"crash on a code defect", "CrashLoop", failingH, rcaPanic,
			false},
		{"restarts without a connection error", "Restarting", failingH,
			"exit status 1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			effect := probePortCase(f, tc.mode, tc.health, tc.text)
			c, ok := f.explain().CauseOf(effect)
			got := ok && c.Row == "probe-port-mismatch"
			if got != tc.want {
				t.Fatalf("probe-port-mismatch = %v (cause %v, row %q), "+
					"want %v", got, c.Root, c.Row, tc.want)
			}
		})
	}
}
