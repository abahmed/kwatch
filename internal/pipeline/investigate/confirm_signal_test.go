package investigate

import (
	"strconv"
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func crashProof(
	t *testing.T, attrs map[string]inventory.Value, mode string,
) string {
	t.Helper()
	root := inventory.CoreID(kube.KindDeployment, "shop", "api")
	return confirmedFor(t, failedRunModel(t, attrs), root, mode)
}

// An exit by signal is how the run ended, not why: 137 and 143 are what
// an OOM kill or a failed liveness probe look like too, so the exit code
// alone confirms no crash.
func TestConfirmCrashExitNeedsMoreThanASignalExitCode(t *testing.T) {
	for _, code := range []float64{137, 143, 139, 129, 255} {
		t.Run(strconvCode(code), func(t *testing.T) {
			unknown := map[string]inventory.Value{
				kube.AttrLastExitCode: num(code)}
			if got := crashProof(t, unknown, "Error"); got != "" {
				t.Fatalf("a signal exit with no reason confirmed %q", got)
			}
			oom := map[string]inventory.Value{
				kube.AttrLastExitCode: num(code),
				kube.AttrLastReason:   text("OOMKilled")}
			if got := crashProof(t, oom, "Error"); got != "" &&
				got == "the last run of "+crashContainer.Name+
					" exited with code "+strconvCode(code) {
				t.Fatalf("an OOM kill confirmed a crash: %q", got)
			}
			probe := map[string]inventory.Value{
				kube.AttrLastExitCode:     num(code),
				kube.AttrLastReason:       text("Error"),
				kube.AttrLivenessFailures: num(3)}
			if got := crashProof(t, probe, "Error"); got != "" {
				t.Fatalf("a probe kill confirmed a crash: %q", got)
			}
		})
	}
}

// With a known non-OOM reason and no probe evidence, a signal exit such
// as a segfault still confirms the crash.
func TestConfirmCrashExitAcceptsAnUnexplainedSignal(t *testing.T) {
	attrs := map[string]inventory.Value{
		kube.AttrLastExitCode: num(139),
		kube.AttrLastReason:   text("Error")}
	if got := crashProof(t, attrs, "Error"); got == "" {
		t.Fatal("a segfault with ruled-out causes was not confirmed")
	}
}

// Ordinary non-zero exits confirm as before.
func TestConfirmCrashExitStillAcceptsPlainExitCodes(t *testing.T) {
	attrs := map[string]inventory.Value{kube.AttrLastExitCode: num(2)}
	if got := crashProof(t, attrs, "Error"); got == "" {
		t.Fatal("exit code 2 was not confirmed")
	}
}

func strconvCode(code float64) string {
	return strconv.Itoa(int(code))
}
