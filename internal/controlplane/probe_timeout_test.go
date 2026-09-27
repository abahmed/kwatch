package controlplane

import (
	"context"
	"testing"
	"time"
)

func TestWithProbeTimeoutGivesEachProbeItsOwnDeadline(t *testing.T) {
	parent := context.Background()
	var first, second time.Time
	withProbeTimeout(parent, func(ctx context.Context) {
		first, _ = ctx.Deadline()
	})
	withProbeTimeout(parent, func(ctx context.Context) {
		second, _ = ctx.Deadline()
	})
	if first.IsZero() || second.IsZero() || second.Before(first) {
		t.Fatalf("deadlines first=%s second=%s", first, second)
	}
}
