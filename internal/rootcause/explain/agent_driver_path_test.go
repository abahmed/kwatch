package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const csiSocketDir = "/var/lib/kubelet/plugins/"

// A registrar is given the driver's name inside the socket path it
// registers (DRIVER_REG_SOCK_PATH), not as a value of its own: the
// plugin that names the driver there is still the one the error names.
func TestAgentDriverGateReadsTheDriverFromASocketPath(t *testing.T) {
	f := newFixture(t)
	_, csi, victim := csiCase(f,
		csiSocketDir+"ebs.csi.example.com/csi.sock")
	f.fail(csi, detection.ModeCrashLoop, failingH, 1, "")

	c := requireCause(t, f.explain(), victim, csi.String())
	if c.Row != agentNamedRow {
		t.Fatalf("row = %s, want %s", c.Row, agentNamedRow)
	}
}

// The registration socket file is named after the driver.
func TestAgentDriverGateReadsTheDriverFromARegistrationSocket(t *testing.T) {
	f := newFixture(t)
	_, csi, victim := csiCase(f,
		"/registration/ebs.csi.example.com-reg.sock")
	f.fail(csi, detection.ModeCrashLoop, failingH, 1, "")

	requireCause(t, f.explain(), victim, csi.String())
}

// Only a whole path segment is the driver's name.
func TestAgentDriverGateSocketPathNeedsAWholeSegment(t *testing.T) {
	for _, path := range []string{
		csiSocketDir + "other.csi.example.com/csi.sock",
		csiSocketDir + "notebs.csi.example.com/csi.sock",
		csiSocketDir + "ebs.csi.example.com.evil/csi.sock",
	} {
		f := newFixture(t)
		_, csi, victim := csiCase(f, path)
		f.fail(csi, detection.ModeCrashLoop, failingH, 1, "")

		if c, ok := f.explain().CauseOf(victim); ok && c.Root == csi {
			t.Fatalf("%s blamed for the driver: %+v", path, c)
		}
	}
}

// With no name on its pods, the DaemonSet's own arguments may quote it.
func TestAgentDriverGateFallsBackToTheDaemonSetArguments(t *testing.T) {
	f := newFixture(t)
	_, csi, victim := csiCase(f, "")
	f.setAttrs(inventory.CoreID(kube.KindDaemonSet, "kube-system", "csi"),
		attrs{kube.AttrTemplateQuoted: inventory.Text(
			"ebs.csi.example.com/csi.sock")})
	f.fail(csi, detection.ModeCrashLoop, failingH, 1, "")

	c := requireCause(t, f.explain(), victim, csi.String())
	if c.Row != agentNamedRow {
		t.Fatalf("row = %s, want %s", c.Row, agentNamedRow)
	}
}

// Another driver's name in the DaemonSet's arguments is not this one.
func TestAgentDriverGateFallbackNeedsTheSameDriver(t *testing.T) {
	f := newFixture(t)
	_, csi, victim := csiCase(f, "")
	f.setAttrs(inventory.CoreID(kube.KindDaemonSet, "kube-system", "csi"),
		attrs{kube.AttrTemplateQuoted: inventory.Text(
			"other.csi.example.com/csi.sock")})
	f.fail(csi, detection.ModeCrashLoop, failingH, 1, "")

	if c, ok := f.explain().CauseOf(victim); ok && c.Root == csi {
		t.Fatalf("a plugin for another driver blamed: %+v", c)
	}
}
