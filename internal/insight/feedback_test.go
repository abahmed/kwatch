package insight

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/model"
)

func TestFeedbackRequiresObservationsBeforeBias(t *testing.T) {
	store := NewFeedbackStore()
	inc := &model.Incident{Subject: model.Subject{Key: "pod/prod/api|crash", Reason: "CrashLoopBackOff"}}
	for i := 0; i < 2; i++ {
		store.Observe(inc, model.ActionCreate, "rollout")
	}
	if got := store.Bias("crashloopbackoff|rollout"); got != 0 {
		t.Fatalf("bias before warmup = %v", got)
	}
	store.Observe(inc, model.ActionResolved, "rollout")
	store.Observe(inc, model.ActionCreate, "rollout")
	if got := store.Bias("crashloopbackoff|rollout"); got >= 0 {
		t.Fatalf("recurrence should reduce bias, got %v", got)
	}
}

func TestFeedbackSnapshotUsesFingerprintAsTieBreaker(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	store := NewFeedbackStoreWithClock(clock.Func(func() time.Time {
		return now
	}))
	store.Observe(
		&model.Incident{Subject: model.Subject{
			Key: "pod/prod/api|crash", Reason: "CrashLoopBackOff",
		}},
		model.ActionCreate,
		"z-pattern",
	)
	store.Observe(
		&model.Incident{Subject: model.Subject{
			Key: "pod/prod/api|crash", Reason: "CrashLoopBackOff",
		}},
		model.ActionCreate,
		"a-pattern",
	)

	records := store.Snapshot()
	if len(records) != 2 {
		t.Fatalf("Snapshot() returned %d records, want 2", len(records))
	}
	if records[0].Fingerprint >= records[1].Fingerprint {
		t.Fatalf(
			"Snapshot() order = %q, %q; want fingerprint order",
			records[0].Fingerprint,
			records[1].Fingerprint,
		)
	}
}
