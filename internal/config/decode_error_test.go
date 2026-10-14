package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A secret read from a file into a field that is not a string must not be
// echoed by the decode error, in the error or in the log line.
func TestDecodeErrorDoesNotEchoSecretValue(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	require.NoError(t, os.WriteFile(secret, []byte("hunter2-secret"), 0o600))
	cfg := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfg,
		[]byte("healthCheck:\n  port: ${file:"+secret+"}\n"), 0o600))
	t.Setenv("CONFIG_FILE", cfg)

	_, err := parseConfigFile()

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "hunter2")
	assert.Contains(t, err.Error(), "cannot unmarshal")
	assert.Contains(t, err.Error(), "<value>")
}
