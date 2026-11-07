package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/license"
)

func TestNewLicenseSetupReadsTheConfiguration(t *testing.T) {
	cfg := config.DefaultConfig()
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	setup := newLicenseSetup(config.RuntimeConfigFor(cfg), start)
	require.Empty(t, setup.key)
	require.Equal(t, start, setup.processStartTime())
	require.Equal(t, license.DefaultEndpoint, setup.endpoint)
	require.Nil(t, setup.verifier, "production trusts only compiled-in keys")

	cfg.License = config.License{Key: "KW1-x", Endpoint: "http://test"}
	setup = newLicenseSetup(config.RuntimeConfigFor(cfg), start)
	require.Equal(t, "KW1-x", setup.key)
	require.Equal(t, "http://test", setup.endpoint)

	cfg.License.Endpoint = ""
	setup = newLicenseSetup(config.RuntimeConfigFor(cfg), start)
	require.Equal(t, license.DefaultEndpoint, setup.endpoint)
}
