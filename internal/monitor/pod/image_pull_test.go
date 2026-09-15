package pod

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImagePullMsgHintRateLimit(t *testing.T) {
	assert.Contains(
		t,
		ImagePullMessageHint("toomanyrequests: pull limit", false),
		"rate limit",
	)
	assert.Contains(
		t,
		ImagePullMessageHint("rate limit exceeded", false),
		"rate limit",
	)
}

func TestImagePullMsgHintPullQPS(t *testing.T) {
	assert.Contains(t, ImagePullMessageHint("pull qps exceeded", false), "QPS")
}

func TestImagePullMsgHintAuth(t *testing.T) {
	assert.Contains(
		t,
		ImagePullMessageHint("authentication required", false),
		"authentication",
	)
	assert.Contains(
		t,
		ImagePullMessageHint("unauthorized: access denied", false),
		"authentication",
	)
	assert.Contains(
		t,
		ImagePullMessageHint("denied: access forbidden", false),
		"authentication",
	)
	assert.Contains(
		t,
		ImagePullMessageHint("no pull access", false),
		"authentication",
	)
}

func TestImagePullMsgHintNotFound(t *testing.T) {
	withSecrets := ImagePullMessageHint("not found: nginx:latest", true)
	assert.Contains(t, withSecrets, "not found")
	assert.Contains(t, withSecrets, "registry")
	withoutSecrets := ImagePullMessageHint("manifest unknown", false)
	assert.Contains(t, withoutSecrets, "not found")
	assert.NotContains(t, withoutSecrets, "registry")
}

func TestImagePullMsgHintTimeout(t *testing.T) {
	assert.Contains(
		t,
		ImagePullMessageHint("context deadline exceeded", false),
		"timed out",
	)
	assert.Contains(t, ImagePullMessageHint("i/o timeout", false), "timed out")
}

func TestImagePullMsgHintConnRefused(t *testing.T) {
	assert.Contains(
		t,
		ImagePullMessageHint("connection refused", false),
		"refused",
	)
	assert.Contains(t, ImagePullMessageHint("connection reset", false), "refused")
}

func TestImagePullMsgHintNoRoute(t *testing.T) {
	assert.Contains(t, ImagePullMessageHint("no route to host", false), "route")
	assert.Contains(
		t,
		ImagePullMessageHint("network is unreachable", false),
		"route",
	)
}

func TestImagePullMsgHintDNS(t *testing.T) {
	assert.Contains(t, ImagePullMessageHint("no such host", false), "DNS")
	assert.Contains(
		t,
		ImagePullMessageHint("dial tcp: lookup registry.example.com", false),
		"DNS",
	)
}

func TestImagePullMsgHintTLS(t *testing.T) {
	assert.Contains(t, ImagePullMessageHint("tls handshake error", false), "TLS")
	assert.Contains(t, ImagePullMessageHint("certificate expired", false), "TLS")
}

func TestImagePullMsgHintNoMatch(t *testing.T) {
	assert.Equal(t, "", ImagePullMessageHint("some random error", false))
}

func TestNeedsRegistryAuth(t *testing.T) {
	assert.False(t, NeedsRegistryAuth("nginx"))
	assert.False(t, NeedsRegistryAuth("nginx:latest"))
	assert.False(t, NeedsRegistryAuth("library/nginx"))
	assert.False(t, NeedsRegistryAuth("myuser/myimage"))
	assert.True(t, NeedsRegistryAuth("gcr.io/myproject/myimage"))
	assert.True(t, NeedsRegistryAuth("myregistry.io:5000/myimage"))
	assert.True(t, NeedsRegistryAuth("docker.io/user/repo"))
	assert.False(t, NeedsRegistryAuth(""))
}
