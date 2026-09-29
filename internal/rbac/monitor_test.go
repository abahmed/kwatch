package rbac

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

// reviewer answers access reviews: deny lists resources and URLs refused.
func reviewer(deny map[string]bool, fail error) *fake.Clientset {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			if fail != nil {
				return true, nil, fail
			}
			review := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview)
			key := ""
			if attrs := review.Spec.ResourceAttributes; attrs != nil {
				key = attrs.Resource
				if attrs.Subresource != "" {
					key += "/" + attrs.Subresource
				}
			} else {
				key = review.Spec.NonResourceAttributes.Path
			}
			review.Status.Allowed = !deny[key]
			return true, review, nil
		})
	return client
}

func sweep(t *testing.T, deny map[string]bool, fail error) Status {
	t.Helper()
	m := NewMonitor(reviewer(deny, fail), Checks("kwatch", true),
		clock.RealClock{}, func(Status) {})
	return m.Sweep(context.Background())
}

func TestSweepAllowedEverything(t *testing.T) {
	status := sweep(t, nil, nil)
	assert.Empty(t, status.Missing)
	assert.False(t, status.Unavailable)
	assert.False(t, status.LastCheck.IsZero())
}

func TestSweepSeparatesRequiredAndOptional(t *testing.T) {
	status := sweep(t, map[string]bool{"pods/log": true, "/readyz": true}, nil)
	require.Len(t, status.Missing, 2)
	assert.False(t, status.RequiredMissing())

	status = sweep(t, map[string]bool{"pods": true}, nil)
	assert.True(t, status.RequiredMissing())
}

func TestSweepReportsUnavailableReviewAPI(t *testing.T) {
	status := sweep(t, nil, errors.New("connection refused"))
	assert.True(t, status.Unavailable)
}

func TestChecksIncludeLeaseAndOptionalCRD(t *testing.T) {
	has := func(checks []kube.Access, name string) bool {
		for _, c := range checks {
			if c.Resource.Name == name {
				return true
			}
		}
		return false
	}
	assert.True(t, has(Checks("kwatch", false), "leases"))
	assert.False(t, has(Checks("kwatch", false), "kwatchconfigs"))
	assert.True(t, has(Checks("kwatch", true), "kwatchconfigs"))
}

func TestStartReportsUntilCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reports := 0
	m := NewMonitor(reviewer(nil, nil), Checks("kwatch", false),
		clock.RealClock{}, func(Status) {
			reports++
			cancel()
		})
	require.NoError(t, m.Start(ctx))
	assert.Equal(t, 1, reports)
}
