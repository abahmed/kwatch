package kube

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var podsResource = schema.GroupResource{Resource: "pods"}

func TestEvidenceReadErrorClasses(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		denied bool
		absent bool
	}{
		{"nil", nil, false, false},
		{"forbidden", apierrors.NewForbidden(podsResource, "p",
			errors.New("no")), true, false},
		{"unauthorized", apierrors.NewUnauthorized("expired"),
			true, false},
		{"not found", apierrors.NewNotFound(podsResource, "p"),
			false, true},
		{"bad request", apierrors.NewBadRequest(
			"container is not valid"), false, true},
		{"server error", apierrors.NewInternalError(errors.New("x")),
			false, false},
		{"timeout", apierrors.NewTimeoutError("slow", 1), false, false},
		{"plain", errors.New("boom"), false, false},
		{"wrapped forbidden", fmt.Errorf("read: %w",
			apierrors.NewForbidden(podsResource, "p", errors.New("no"))),
			true, false},
		{"wrapped not found", fmt.Errorf("read: %w",
			apierrors.NewNotFound(podsResource, "p")), false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.denied, IsDenied(c.err))
			assert.Equal(t, c.absent, IsAbsent(c.err))
		})
	}
}
