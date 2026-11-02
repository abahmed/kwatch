package kube_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func forbidLogs(r *crashRig) {
	r.logs.err = apierrors.NewForbidden(
		schema.GroupResource{Resource: "pods/log"}, "p1", nil)
}

// A refused pods/log read is asked again only after the backoff, not
// every round: one refusal per 5 minutes in the audit log, not 120.
func TestCrashLogRoundBacksOffWhenForbidden(t *testing.T) {
	r := newCrashRig(t)
	forbidLogs(r)
	r.crash(t, "p1", 1)
	ctx := context.Background()

	assert.False(t, r.round.PodLogsForbidden())
	r.round.Round(ctx)
	assert.True(t, r.round.PodLogsForbidden())
	for range 9 {
		r.round.Round(ctx)
	}
	assert.Len(t, r.logs.reads, 1, "no read during the backoff")
	r.round.Round(ctx)
	assert.Len(t, r.logs.reads, 2, "one retry once the backoff is over")
	assert.Empty(t, r.submitted, "a refusal is never an observation")
}

func TestCrashLogRoundRetriesEarlyWhenTheCrashedSetChanges(t *testing.T) {
	r := newCrashRig(t)
	forbidLogs(r)
	r.crash(t, "p1", 1)
	ctx := context.Background()
	r.round.Round(ctx)
	r.round.Round(ctx)
	assert.Len(t, r.logs.reads, 1)

	r.logs.err = nil
	r.crash(t, "p2", 1)
	r.round.Round(ctx)
	assert.Len(t, r.logs.reads, 3, "a new crash earns a try")
	assert.False(t, r.round.PodLogsForbidden(), "a read got through")
}
