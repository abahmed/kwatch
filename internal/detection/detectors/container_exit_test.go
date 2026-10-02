package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestContainerExitCodeModes(t *testing.T) {
	cases := []struct {
		name   string
		code   float64
		reason string
		want   string
		mode   string
	}{
		{"not executable", 126, "Error", reasons.ExitNotExecutable,
			"Exit.NotExecutable"},
		{"command not found", 127, "Error", reasons.ExitCommandNotFound,
			"Exit.CommandNotFound"},
		{"sigkill without oom", 137, "Error", reasons.ExitKilled,
			"Exit.Killed"},
		{"oom kill stays oom", 137, reasons.OOMKilled, reasons.OOMKilled,
			"OOMKilled"},
		{"lost container stays generic", 137, "ContainerStatusUnknown",
			reasons.Error, "Error"},
		{"segfault", 139, "Error", reasons.ExitSegfault, "Exit.Segfault"},
		{"sigterm stays generic", 143, "Error", reasons.Error, "Error"},
		{"plain error stays generic", 1, "Error", reasons.Error, "Error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := newTestModel()
			id := kube.ContainerID("default", "app", "main")
			observeEntity(model, id, podNodeNow,
				map[string]inventory.Value{
					kube.AttrState:       inventory.Text("terminated"),
					kube.AttrStateReason: inventory.Text(tc.reason),
					kube.AttrExitCode:    inventory.Number(tc.code),
				})
			found := classified(Container{}.Detect(
				testDetectorContext(model, podNodeNow),
				entityOf(model, id)))
			require.Len(t, found, 1)
			assert.Equal(t, tc.want, found[0].Reason)
			assert.Equal(t, tc.mode, string(found[0].Mode))
			assert.Equal(t, detection.Failing, found[0].Health)
		})
	}
}

func TestContainerExitCodeInitKeepsInitError(t *testing.T) {
	model := newTestModel()
	id := kube.ContainerID("default", "app", "migrate")
	observeEntity(model, id, podNodeNow, map[string]inventory.Value{
		kube.AttrState:       inventory.Text("terminated"),
		kube.AttrStateReason: inventory.Text("Error"),
		kube.AttrExitCode:    inventory.Number(127),
		kube.AttrInit:        inventory.Bool(true),
	})
	found := Container{}.Detect(testDetectorContext(model, podNodeNow),
		entityOf(model, id))
	require.Len(t, found, 1)
	assert.Equal(t, reasons.InitContainerError, found[0].Reason)
}

func TestContainerCrashLoopNamedAfterExitCode(t *testing.T) {
	cases := []struct {
		name string
		code float64
		want string
	}{
		{"command not found", 127, reasons.ExitCommandNotFound},
		{"segfault", 139, reasons.ExitSegfault},
		{"sigkill stays crash loop", 137, reasons.CrashLoopBackOff},
		{"generic exit stays crash loop", 1, reasons.CrashLoopBackOff},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := newTestModel()
			id := kube.ContainerID("default", "app", "main")
			observeEntity(model, id, podNodeNow,
				map[string]inventory.Value{
					kube.AttrState: inventory.Text("waiting"),
					kube.AttrStateReason: inventory.Text(
						reasons.CrashLoopBackOff),
					kube.AttrLastReason:   inventory.Text("Error"),
					kube.AttrLastExitCode: inventory.Number(tc.code),
				})
			found := Container{}.Detect(
				testDetectorContext(model, podNodeNow), entityOf(model, id))
			require.Len(t, found, 1)
			assert.Equal(t, tc.want, found[0].Reason)
		})
	}
}

func TestContainerWaitingReasonsAdded(t *testing.T) {
	cases := map[string]string{
		reasons.ErrImageNeverPull:  "ImagePull.NeverPull",
		reasons.PreStartHookError:  "Hook.PreStart",
		reasons.PostStartHookError: "Hook.PostStart",
	}
	for reason, mode := range cases {
		model := newTestModel()
		id := kube.ContainerID("default", "app", "main")
		observeEntity(model, id, podNodeNow, map[string]inventory.Value{
			kube.AttrState:       inventory.Text("waiting"),
			kube.AttrStateReason: inventory.Text(reason),
		})
		found := classified(Container{}.Detect(
			testDetectorContext(model, podNodeNow), entityOf(model, id)))
		require.Len(t, found, 1, reason)
		assert.Equal(t, mode, string(found[0].Mode))
		assert.Equal(t, detection.Failing, found[0].Health)
		assert.NotContains(t, found[0].Summary, "("+reason+")")
	}
}

func TestContainerWaitingProgressIsNotAFinding(t *testing.T) {
	model := newTestModel()
	id := kube.ContainerID("default", "app", "main")
	observeEntity(model, id, podNodeNow, map[string]inventory.Value{
		kube.AttrState:       inventory.Text("waiting"),
		kube.AttrStateReason: inventory.Text(reasons.ContainerCreating),
	})
	assert.Empty(t, Container{}.Detect(
		testDetectorContext(model, podNodeNow), entityOf(model, id)))
}
