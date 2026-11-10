package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvironmentMustBeProductionOrNonProduction(t *testing.T) {
	for env, ok := range map[string]bool{
		"": true, "production": true, "non-production": true,
		"Non-Production": true, "staging": false, "nonprod": false,
	} {
		cfg := DefaultConfig()
		cfg.App.Environment = env
		bad := false
		for _, err := range Validate(cfg) {
			bad = bad || strings.Contains(err.Error(), "app.environment")
		}
		if bad == ok {
			t.Errorf("environment %q: rejected=%v", env, bad)
		}
	}
}

// Only non-production turns paging off; the default is production.
func TestNonProductionReachesTheRuntime(t *testing.T) {
	cfg := DefaultConfig()
	assert.False(t, CompileRuntimeConfig(cfg).Application().NonProduction)
	cfg.App.Environment = " non-production "
	assert.True(t, CompileRuntimeConfig(cfg).Application().NonProduction)
	cfg.App.Environment = "production"
	assert.False(t, CompileRuntimeConfig(cfg).Application().NonProduction)
}
