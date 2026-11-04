package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// A Warning liveness failure explains the end of a run only when it came
// just before that end, the container restarted, enough failures were
// seen and the probe got as far as the process.
func TestLivenessFailureKillNeedsAFailureJustBeforeTheExit(t *testing.T) {
	end := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	base := LivenessFailure{Message: "Liveness probe failed: HTTP " +
		"probe failed with statuscode: 500", At: end.Add(-30 * time.Second),
		Count: 3, Threshold: 3, Finished: end, Restarts: 2}
	cases := map[string]struct {
		change func(*LivenessFailure)
		want   bool
	}{
		"just before": {func(*LivenessFailure) {}, true},
		"threshold unknown": {func(f *LivenessFailure) {
			f.Threshold, f.Count = 0, 1
		}, true},
		"below threshold": {func(f *LivenessFailure) { f.Count = 2 },
			false},
		"after the exit": {func(f *LivenessFailure) {
			f.At = end.Add(time.Second)
		}, false},
		"too old": {func(f *LivenessFailure) {
			f.At = end.Add(-LivenessKillLead - time.Second)
		}, false},
		"no restart": {func(f *LivenessFailure) { f.Restarts = 0 }, false},
		"readiness": {func(f *LivenessFailure) {
			f.Message = "Readiness probe failed: HTTP 500"
		}, false},
		"refused": {func(f *LivenessFailure) {
			f.Message = "Liveness probe failed: dial tcp :80: " +
				"connect: connection refused"
		}, false},
		"no end": {func(f *LivenessFailure) { f.Finished = time.Time{} },
			false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := base
			tc.change(&f)
			assert.Equal(t, tc.want, LivenessFailureKill(f))
		})
	}
}
