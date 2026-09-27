package workload

import (
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestJobBackoffLimitAlertsOnlyAfterLimitIsExceeded(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	start := metav1.NewTime(now.Add(-time.Minute))
	job := func(limit, failed int32) *batchv1.Job {
		return &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "j", Namespace: "ns"},
			Spec:       batchv1.JobSpec{BackoffLimit: &limit},
			Status: batchv1.JobStatus{
				StartTime: &start, Active: 1, Failed: failed,
			},
		}
	}
	for _, tc := range []struct {
		name          string
		limit, failed int32
		alert         bool
	}{
		{"no retries allowed and still running", 0, 0, false},
		{"last retry still running", 3, 3, false},
		{"limit exceeded", 3, 4, true},
		{"no retries allowed and one failure", 0, 1, true},
	} {
		got := DetectJobExecutionIssue(job(tc.limit, tc.failed), now) != nil
		if got != tc.alert {
			t.Fatalf("%s: alert=%v, want %v", tc.name, got, tc.alert)
		}
	}
}
