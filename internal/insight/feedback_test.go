package insight

import (
	"testing"

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
