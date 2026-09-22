//go:build e2e

package harness

import "testing"

func TestClassifyVersionRuns(t *testing.T) {
	tests := []struct {
		name     string
		reported VersionRun
		main     VersionRun
		want     ReproductionClass
	}{
		{
			name:     "fixed on main",
			reported: VersionRun{Reproduced: true},
			main:     VersionRun{Reproduced: true, Passed: true},
			want:     FixedOnMain,
		},
		{
			name:     "still failing",
			reported: VersionRun{Reproduced: true},
			main:     VersionRun{Reproduced: true},
			want:     StillFailing,
		},
		{
			name:     "regression",
			reported: VersionRun{Reproduced: true, Passed: true},
			main:     VersionRun{Reproduced: true},
			want:     RegressionOnMain,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ClassifyVersionRuns(test.reported, test.main); got != test.want {
				t.Fatalf("class = %q, want %q", got, test.want)
			}
		})
	}
}
