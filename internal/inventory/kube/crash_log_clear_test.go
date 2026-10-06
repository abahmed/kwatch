package kube_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// applySubmitted feeds what the round submitted into the model.
func (r *crashRig) applySubmitted(t *testing.T) {
	t.Helper()
	for _, o := range r.submitted {
		_, err := r.model.Apply(o)
		require.NoError(t, err)
	}
	r.submitted = nil
}

func (r *crashRig) errorLine(id inventory.EntityID) bool {
	e, ok := r.model.Entity(id)
	if !ok {
		return false
	}
	_, has := e.Attribute(kube.AttrLastErrorLine)
	return has
}

func TestCrashLogLineOfAnEarlierRunIsRemovedWhenThereIsNoLog(t *testing.T) {
	for _, err := range []error{
		apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "p1"),
		apierrors.NewBadRequest("previous terminated container not found"),
	} {
		r := newCrashRig(t)
		id := r.crash(t, "p1", 1)
		r.logs.lines["p1/app"] = []string{redisLine}
		r.round.Round(context.Background())
		r.applySubmitted(t)
		require.True(t, r.errorLine(id))

		r.logs.err = err
		r.crash(t, "p1", 2)
		r.round.Round(context.Background())
		r.applySubmitted(t)
		assert.False(t, r.errorLine(id), "the old run's line is stale")
	}
}

func TestCrashLogLineIsRemovedWhenTheLastExitIsClean(t *testing.T) {
	r := newCrashRig(t)
	id := r.crash(t, "p1", 1)
	r.logs.lines["p1/app"] = []string{redisLine}
	r.round.Round(context.Background())
	r.applySubmitted(t)
	require.True(t, r.errorLine(id))

	r.apply(t, id, map[string]inventory.Value{
		kube.AttrRestarts:     inventory.Number(2),
		kube.AttrState:        inventory.Text("running"),
		kube.AttrLastExitCode: inventory.Number(0),
	})
	r.round.Round(context.Background())
	r.applySubmitted(t)
	assert.False(t, r.errorLine(id))

	r.round.Round(context.Background())
	assert.Empty(t, r.submitted, "the removal is submitted once")
}
