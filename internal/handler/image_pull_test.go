package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImagePullMsgHintRateLimit(t *testing.T) {
	assert.Contains(t, imagePullMsgHint("toomanyrequests: pull limit", false), "rate limit")
	assert.Contains(t, imagePullMsgHint("rate limit exceeded", false), "rate limit")
}

func TestImagePullMsgHintPullQPS(t *testing.T) {
	assert.Contains(t, imagePullMsgHint("pull qps exceeded", false), "QPS")
}

func TestImagePullMsgHintAuth(t *testing.T) {
	assert.Contains(t, imagePullMsgHint("authentication required", false), "authentication")
	assert.Contains(t, imagePullMsgHint("unauthorized: access denied", false), "authentication")
	assert.Contains(t, imagePullMsgHint("denied: access forbidden", false), "authentication")
	assert.Contains(t, imagePullMsgHint("no pull access", false), "authentication")
}

func TestImagePullMsgHintNotFound(t *testing.T) {
	withSecrets := imagePullMsgHint("not found: nginx:latest", true)
	assert.Contains(t, withSecrets, "not found")
	assert.Contains(t, withSecrets, "registry")
	withoutSecrets := imagePullMsgHint("manifest unknown", false)
	assert.Contains(t, withoutSecrets, "not found")
	assert.NotContains(t, withoutSecrets, "registry")
}

func TestImagePullMsgHintTimeout(t *testing.T) {
	assert.Contains(t, imagePullMsgHint("context deadline exceeded", false), "timed out")
	assert.Contains(t, imagePullMsgHint("i/o timeout", false), "timed out")
}

func TestImagePullMsgHintConnRefused(t *testing.T) {
	assert.Contains(t, imagePullMsgHint("connection refused", false), "refused")
	assert.Contains(t, imagePullMsgHint("connection reset", false), "refused")
}

func TestImagePullMsgHintNoRoute(t *testing.T) {
	assert.Contains(t, imagePullMsgHint("no route to host", false), "route")
	assert.Contains(t, imagePullMsgHint("network is unreachable", false), "route")
}

func TestImagePullMsgHintDNS(t *testing.T) {
	assert.Contains(t, imagePullMsgHint("no such host", false), "DNS")
	assert.Contains(t, imagePullMsgHint("dial tcp: lookup registry.example.com", false), "DNS")
}

func TestImagePullMsgHintTLS(t *testing.T) {
	assert.Contains(t, imagePullMsgHint("tls handshake error", false), "TLS")
	assert.Contains(t, imagePullMsgHint("certificate expired", false), "TLS")
}

func TestImagePullMsgHintNoMatch(t *testing.T) {
	assert.Equal(t, "", imagePullMsgHint("some random error", false))
}

func TestNeedsRegistryAuth(t *testing.T) {
	assert.False(t, needsRegistryAuth("nginx"))
	assert.False(t, needsRegistryAuth("nginx:latest"))
	assert.False(t, needsRegistryAuth("library/nginx"))
	assert.False(t, needsRegistryAuth("myuser/myimage"))
	assert.True(t, needsRegistryAuth("gcr.io/myproject/myimage"))
	assert.True(t, needsRegistryAuth("myregistry.io:5000/myimage"))
	assert.True(t, needsRegistryAuth("docker.io/user/repo"))
	assert.False(t, needsRegistryAuth(""))
}
