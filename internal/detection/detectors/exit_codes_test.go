package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The decoder is the one place exit codes are read: every code maps to
// the reason, the wording and the crash-loop naming below.
func TestExitCodeDecoder(t *testing.T) {
	cases := []struct {
		code      float64
		reason    string
		summary   string
		crashLoop bool
	}{
		{126, reasons.ExitNotExecutable, "cannot run its command: the " +
			"file is not executable (exit code 126)", true},
		{127, reasons.ExitCommandNotFound, "cannot run its command: the " +
			"command was not found (exit code 127)", true},
		{132, reasons.ExitIllegalInstruction,
			"crashed with an illegal instruction (exit code 132)", true},
		{134, reasons.ExitAborted,
			"aborted with SIGABRT (exit code 134)", true},
		{135, reasons.ExitBusError,
			"crashed with a bus error (exit code 135)", true},
		{136, reasons.ExitFloatingPoint, "crashed with an arithmetic " +
			"error, SIGFPE (exit code 136)", true},
		{137, reasons.ExitKilled, "was killed by SIGKILL (exit code " +
			"137) without running out of memory", false},
		{139, reasons.ExitSegfault,
			"crashed (exit code 139, signal SIGSEGV)", true},
		{255, reasons.Error, "exited with code 255 (a status of -1 " +
			"or out of range)", false},
	}
	for _, tc := range cases {
		got, ok := exitMeaningFor(tc.code)
		require.True(t, ok, tc.code)
		assert.Equal(t, tc.reason, got.reason, tc.code)
		assert.Equal(t, tc.summary, got.summary, tc.code)
		assert.Equal(t, tc.crashLoop, got.crashLoop, tc.code)
	}
}

// Generic application errors and SIGTERM have no decoded meaning.
func TestExitCodeDecoderLeavesGenericCodes(t *testing.T) {
	for _, code := range []float64{0, 1, 2, 130, 143, 254} {
		_, ok := exitMeaningFor(code)
		assert.False(t, ok, code)
	}
}

func TestExitSignalCodesAre128PlusSignal(t *testing.T) {
	assert.Equal(t, 132, exitSIGILL)
	assert.Equal(t, 134, exitSIGABRT)
	assert.Equal(t, 135, exitSIGBUS)
	assert.Equal(t, 136, exitSIGFPE)
	assert.Equal(t, 137, exitSIGKILL)
	assert.Equal(t, 139, exitSIGSEGV)
	assert.Equal(t, 143, exitSIGTERM)
}

func TestContainerNewExitCodesTerminatedAndCrashLoop(t *testing.T) {
	cases := []struct {
		code   float64
		reason string
		mode   string
	}{
		{132, reasons.ExitIllegalInstruction, "Exit.IllegalInstruction"},
		{134, reasons.ExitAborted, "Exit.Aborted"},
		{135, reasons.ExitBusError, "Exit.BusError"},
		{136, reasons.ExitFloatingPoint, "Exit.FloatingPoint"},
	}
	for _, tc := range cases {
		model := newTestModel()
		id := kube.ContainerID("default", "app", "main")
		observeEntity(model, id, podNodeNow, map[string]inventory.Value{
			kube.AttrState:       inventory.Text("terminated"),
			kube.AttrStateReason: inventory.Text("Error"),
			kube.AttrExitCode:    inventory.Number(tc.code),
		})
		found := classified(Container{}.Detect(
			testDetectorContext(model, podNodeNow), entityOf(model, id)))
		require.Len(t, found, 1, tc.code)
		assert.Equal(t, tc.reason, found[0].Reason)
		assert.Equal(t, tc.mode, string(found[0].Mode))

		loop := newTestModel()
		observeEntity(loop, id, podNodeNow, map[string]inventory.Value{
			kube.AttrState: inventory.Text("waiting"),
			kube.AttrStateReason: inventory.Text(
				reasons.CrashLoopBackOff),
			kube.AttrLastReason:   inventory.Text("Error"),
			kube.AttrLastExitCode: inventory.Number(tc.code),
		})
		found = Container{}.Detect(
			testDetectorContext(loop, podNodeNow), entityOf(loop, id))
		require.Len(t, found, 1, tc.code)
		assert.Equal(t, tc.reason, found[0].Reason)
	}
}

func TestContainerExit255KeepsGenericReason(t *testing.T) {
	model := newTestModel()
	id := kube.ContainerID("default", "app", "main")
	observeEntity(model, id, podNodeNow, map[string]inventory.Value{
		kube.AttrState:       inventory.Text("terminated"),
		kube.AttrStateReason: inventory.Text("Error"),
		kube.AttrExitCode:    inventory.Number(255),
	})
	found := classified(Container{}.Detect(
		testDetectorContext(model, podNodeNow), entityOf(model, id)))
	require.Len(t, found, 1)
	assert.Equal(t, reasons.Error, found[0].Reason)
	assert.Contains(t, found[0].Summary, "exited with code 255")
}

// A runtime that ends with 139 or 134 on an unhandled exception is worded
// after the exception its log shows, not as a segfault; the reason and
// so the mode stay the exit code's.
func TestContainerExitWordsUnhandledException(t *testing.T) {
	cases := []struct {
		code      float64
		line, has string
	}{
		{139, "Unhandled exception. StackExchange.Redis." +
			"RedisConnectionException: It was not possible to connect",
			"crashed with an unhandled exception (exit code 139)"},
		{134, `Exception in thread "main" java.lang.IllegalStateException`,
			"crashed with an unhandled exception (exit code 134)"},
		{139, "", "crashed (exit code 139, signal SIGSEGV)"},
		{139, "panic: runtime error: invalid memory address [signal " +
			"SIGSEGV: segmentation violation]",
			"crashed (exit code 139, signal SIGSEGV)"},
		{134, "", "aborted with SIGABRT (exit code 134)"},
	}
	for _, tc := range cases {
		meaning, ok := exitMeaningFor(tc.code)
		require.True(t, ok)
		e := inventory.Entity{ID: kube.ContainerID("ns", "p", "app"),
			Attributes: map[string]inventory.Attribute{}}
		if tc.line != "" {
			e.Attributes[kube.AttrLastErrorLine] = inventory.Attribute{
				Value: inventory.Text(tc.line)}
		}
		assert.Equal(t, tc.has, exitSummary(meaning, e), tc.line)
	}
}

// A crash loop on 139 whose previous log shows the exception is worded
// after it, with the mode the code names.
func TestContainerCrashLoopExit139WordsException(t *testing.T) {
	model := newTestModel()
	id := kube.ContainerID("web", "api", "app")
	observeEntity(model, id, podNodeNow, map[string]inventory.Value{
		kube.AttrState: inventory.Text("waiting"),
		kube.AttrStateReason: inventory.Text(
			reasons.CrashLoopBackOff),
		kube.AttrLastReason:   inventory.Text("Error"),
		kube.AttrLastExitCode: inventory.Number(139),
		kube.AttrRestarts:     inventory.Number(3),
		kube.AttrLastErrorLine: inventory.Text("Unhandled exception. " +
			"System.InvalidOperationException: boom"),
	})
	found := classified(Container{}.Detect(
		testDetectorContext(model, podNodeNow), entityOf(model, id)))
	require.Len(t, found, 1)
	assert.Equal(t, "Exit.Segfault", string(found[0].Mode))
	assert.Equal(t, "Container crashed with an unhandled exception "+
		"(exit code 139) and is restarting", found[0].Summary)
}
