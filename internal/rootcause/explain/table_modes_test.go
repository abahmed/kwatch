package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
)

// pseudoModes are the modes explain reads itself, for candidates that
// have no finding of their own.
var pseudoModes = []detection.Mode{
	ModeChanged, ModeSpecChanged, ModeScaled, ModeCreated, ModeDeleted,
	ModeMissing, ModeMembersFailing, ModeRejectsNodes, ModeResolution,
	ModeMetricsUnserved, ModeWebhookTimeout, ModeWebhookCallFailed,
	ModeEndpointFailing, ModeSharedSignature, ModeMemoryTooLow,
	ModeProbePortMismatch, ModeStartupBudgetShort,
	pullAuth, pullRateLimit, pullServer, pullTLS, pullNetwork, pullStatus,
}

// TestTableModesAreKnown checks every mode a row names is one some
// finding or pseudo mode can carry, so a misspelt mode fails here
// instead of silently matching nothing.
func TestTableModesAreKnown(t *testing.T) {
	for _, row := range Table() {
		for _, side := range []Side{row.Cause, row.Effect} {
			modes := append(append([]detection.Mode(nil),
				side.Modes...), side.NotModes...)
			for _, mode := range modes {
				if !detection.KnownMode(mode) && !isPseudo(mode) {
					t.Errorf("row %s names unknown mode %q", row.Name, mode)
				}
			}
		}
	}
}

func TestTableModeCheckCatchesTypos(t *testing.T) {
	for _, typo := range []detection.Mode{"CrashLop", "Changd", "Image"} {
		if detection.KnownMode(typo) || isPseudo(typo) {
			t.Errorf("%q passes the mode check", typo)
		}
	}
}

// isPseudo reports whether mode is a pseudo mode or a family of one.
func isPseudo(mode detection.Mode) bool {
	for _, pseudo := range pseudoModes {
		if pseudo.Within(mode) {
			return true
		}
	}
	return false
}
