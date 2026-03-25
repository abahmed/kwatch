package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/abahmed/kwatch/config"
	"github.com/stretchr/testify/assert"
)

func TestGetKubeconfigPath(t *testing.T) {
	assert := assert.New(t)

	path := getKubeconfigPath()
	assert.NotEmpty(path)
	assert.Contains(path, ".kube/config")
}

func TestGetKubeconfigPathFromEnv(t *testing.T) {
	assert := assert.New(t)

	os.Setenv("KUBECONFIG", "/custom/kubeconfig")
	defer os.Unsetenv("KUBECONFIG")

	path := getKubeconfigPath()
	assert.Equal("/custom/kubeconfig", path)
}

func TestGetKubeconfigPathDefault(t *testing.T) {
	assert := assert.New(t)

	os.Unsetenv("KUBECONFIG")

	path := getKubeconfigPath()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".kube", "config")
	assert.Equal(expected, path)
}

func TestCreateClientInvalidKubeconfig(t *testing.T) {
	assert := assert.New(t)

	os.Setenv("KUBECONFIG", "/nonexistent/kubeconfig")
	defer os.Unsetenv("KUBECONFIG")

	cfg := &config.App{}
	_, err := CreateClient(cfg)
	assert.NotNil(err)
	assert.Contains(err.Error(), "cannot build kubernetes out of cluster config")
}

func TestCreateClientInvalidKubeconfigContent(t *testing.T) {
	assert := assert.New(t)

	tmpFile, err := os.CreateTemp("", "kubeconfig-*")
	assert.Nil(err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString("invalid yaml content")
	assert.Nil(err)
	tmpFile.Close()

	os.Setenv("KUBECONFIG", tmpFile.Name())
	defer os.Unsetenv("KUBECONFIG")

	cfg := &config.App{}
	_, err = CreateClient(cfg)
	assert.NotNil(err)
}

func TestCreateClientValidKubeconfig(t *testing.T) {
	assert := assert.New(t)

	kubeconfigContent := `apiVersion: v1
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
	tmpFile, err := os.CreateTemp("", "kubeconfig-*")
	assert.Nil(err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(kubeconfigContent)
	assert.Nil(err)
	tmpFile.Close()

	os.Setenv("KUBECONFIG", tmpFile.Name())
	defer os.Unsetenv("KUBECONFIG")

	cfg := &config.App{}
	client, err := CreateClient(cfg)
	assert.Nil(err)
	assert.NotNil(client)
}

func TestCreateClientWithProxyURL(t *testing.T) {
	assert := assert.New(t)

	kubeconfigContent := `apiVersion: v1
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
	tmpFile, err := os.CreateTemp("", "kubeconfig-*")
	assert.Nil(err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(kubeconfigContent)
	assert.Nil(err)
	tmpFile.Close()

	os.Setenv("KUBECONFIG", tmpFile.Name())
	defer os.Unsetenv("KUBECONFIG")

	cfg := &config.App{
		ProxyURL: "http://proxy:8080",
	}
	client, err := CreateClient(cfg)
	assert.Nil(err)
	assert.NotNil(client)
}
