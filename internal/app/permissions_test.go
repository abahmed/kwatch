package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/rbac"
)

func TestPermissionState(t *testing.T) {
	optional := kube.Access{Verb: "get"}
	required := kube.Access{Verb: "list", Required: true}
	for _, tc := range []struct {
		status rbac.Status
		state  string
		reason string
	}{
		{rbac.Status{}, "running", ""},
		{rbac.Status{Unavailable: true}, "degraded", "api_unavailable"},
		{rbac.Status{Missing: []kube.Access{optional}}, "degraded",
			"optional_permission_denied"},
		{rbac.Status{Missing: []kube.Access{optional, required}},
			"degraded", "permission_denied"},
	} {
		state, reason := permissionState(tc.status)
		assert.Equal(t, tc.state, state)
		assert.Equal(t, tc.reason, reason)
	}
}
