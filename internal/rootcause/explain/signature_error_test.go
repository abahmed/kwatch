package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func errorFinding(id inventory.EntityID, text string) detection.Finding {
	return detection.Finding{Entity: id, Mode: "CrashLoop.Panic",
		Evidence: []detection.Evidence{{Label: "error", Value: text}}}
}

// The record keeps the error one of the pods wrote, not the normalised
// signature, and picks the same one every time.
func TestSharedErrorIsAFailingPodsOwnText(t *testing.T) {
	first := inventory.CoreID(kube.KindPod, "shop", "api-a")
	second := inventory.CoreID(kube.KindPod, "shop", "web-b")
	text := "panic: License key rejected for tenant 4711"
	other := "panic: License key rejected for tenant 99"
	signature := signatureName("CrashLoop.Panic", normalizeSignature(text))
	snap := Snapshot{Findings: map[inventory.EntityID][]detection.Finding{
		second: {errorFinding(second, other)},
		first:  {errorFinding(first, text)},
	}}
	cause := Cause{Root: inventory.CoreID(KindFailureSignature, "", signature)}

	if got := snap.sharedError(cause); got != text {
		t.Fatalf("sharedError = %q, want %q", got, text)
	}
	cause.Root = first
	if got := snap.sharedError(cause); got != "" {
		t.Fatalf("only a signature root has one, got %q", got)
	}
}
