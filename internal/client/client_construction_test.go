package client

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
)

func TestNewClientSetSharesApplicationConfiguration(t *testing.T) {
	kubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://localhost:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`
	file, err := os.CreateTemp("", "kwatch-kubeconfig-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(file.Name()) })
	_, err = file.WriteString(kubeconfig)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	t.Setenv("KUBECONFIG", file.Name())

	clients, err := NewClientSet(&config.App{ProxyURL: "http://proxy:8080"})
	require.NoError(t, err)
	require.NotNil(t, clients.Kubernetes)
	require.NotNil(t, clients.Dynamic)
	require.NotNil(t, clients.Discovery)
	require.NotNil(t, clients.REST)
	require.NotNil(t, clients.HTTP)
	require.NotNil(t, clients.Resolver)
}
