package kubeclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
)

func TestGetNamespaceFromEnv(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "custom-namespace")

	assert.Equal(t, "custom-namespace", GetNamespace())
}

func TestGetNamespaceDefault(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "")

	assert.Equal(t, "kwatch", GetNamespace())
}

func TestNewHTTPClient(t *testing.T) {
	client, err := NewHTTPClient(config.ApplicationRuntime{})

	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, DefaultHTTPTimeout, client.Timeout)
	assert.NotNil(t, client.Transport)
}
