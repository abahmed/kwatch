package scenarios

import (
	"strings"
	"testing"
)

// The message of a bad release names what its revision changed, and the
// order of several edits is the order of likelihood.
func TestRevisionDiffMessagesNameTheChange(t *testing.T) {
	tests := []struct {
		scenario string
		want     []string
		not      []string
	}{
		{"bad-config-value-change", []string{
			"Only change in revision 14: env DB_HOST db-old → db-new " +
				"(by alice); pods crash-looping since."}, nil},
		{"memory-limit-lowered-revision", []string{
			"Only change in revision 14: memory limit 512Mi → 256Mi " +
				"(by alice); pods OOMKilled since."}, nil},
		{"image-and-env-change-ranking", []string{
			"Changes in revision 14, likeliest culprit first: image " +
				"registry.example.com/checkout:5.1 → " +
				"registry.example.com/checkout:5.2; env LOG_LEVEL unset " +
				"→ debug; readiness probe",
			"Also changed before it broke: bob changed config map " +
				"checkout-config at 10:02."},
			[]string{"unrelated-flags"}},
	}
	for _, tc := range tests {
		t.Run(tc.scenario, func(t *testing.T) {
			notes := scenarioNotes(t, tc.scenario)
			for _, want := range tc.want {
				if !strings.Contains(notes, want) {
					t.Errorf("missing %q in:\n%s", want, notes)
				}
			}
			for _, not := range tc.not {
				if strings.Contains(notes, not) {
					t.Errorf("must not name %q in:\n%s", not, notes)
				}
			}
		})
	}
}
