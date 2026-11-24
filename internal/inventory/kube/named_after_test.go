package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNamedAfterNeedsAStemAndASeparator(t *testing.T) {
	assert.True(t, NamedAfter("policy-tls", "policy-controller"))
	assert.True(t, NamedAfter("policy.default.svc", "policy-webhook"))
	assert.False(t, NamedAfter("policyx-tls", "policy-controller"))
	assert.False(t, NamedAfter("my-other-tls", "my-webhook"),
		"a short stem names nothing")
}
