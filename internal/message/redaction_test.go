package message

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactEvidenceRemovesCredentialsAndPrivateAddresses(t *testing.T) {
	input := "Authorization: Bearer secret-token password=hunter2 " +
		"https://example.test/hook?token=abc123 from 10.0.0.7"

	got := RedactEvidence(input)
	assert.NotContains(t, got, "secret-token")
	assert.NotContains(t, got, "hunter2")
	assert.NotContains(t, got, "abc123")
	assert.NotContains(t, got, "10.0.0.7")
	assert.Contains(t, got, "[redacted]")
	assert.Contains(t, got, "[private-address]")
}

func TestRedactEvidencePolicyCanKeepPrivateAddresses(t *testing.T) {
	input := "backend 10.0.0.7 token=abc123"

	got := RedactEvidenceWithPolicy(input, true)

	assert.Contains(t, got, "10.0.0.7")
	assert.NotContains(t, got, "abc123")
}
