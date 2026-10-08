package compose

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// An expired certificate nothing uses breaks nothing: the message states
// the fact and never claims clients are affected.
func TestExpiredCertConsequenceNeedsUse(t *testing.T) {
	tests := []struct {
		name     string
		severity detection.Severity
		want     int
	}{
		{"in use", detection.Critical, 1},
		{"unused", detection.Warning, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := caseFacts{ok: true, lead: detection.Finding{
				Reason: reasons.TLSCertExpired, Severity: tt.severity}}

			got := consequenceSentences(facts)

			if len(got) != tt.want {
				t.Fatalf("got %d consequence sentences, want %d: %v",
					len(got), tt.want, got)
			}
		})
	}
}
