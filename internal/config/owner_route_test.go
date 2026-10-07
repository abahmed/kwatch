package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOwnersAreARouteFieldAndSurviveCompile(t *testing.T) {
	cfg := routeConfig(map[string]interface{}{
		"owners": []interface{}{"payments", "search"},
	})

	providers := CompileRuntimeConfig(cfg).Delivery().Providers()

	require.Len(t, providers, 1)
	require.Len(t, providers[0].Routes, 1, "owners alone make a route")
	assert.Equal(t, []string{"payments", "search"},
		providers[0].Routes[0].Owners)
	for _, warning := range Warnings(cfg) {
		assert.False(t, strings.Contains(warning, "routes[0]"), warning)
	}
}

func TestOwnersAreCopiedOutOfTheRuntimeConfig(t *testing.T) {
	cfg := routeConfig(map[string]interface{}{
		"owners": []interface{}{"payments"},
	})
	runtime := CompileRuntimeConfig(cfg)

	runtime.Delivery().Providers()[0].Routes[0].Owners[0] = "changed"

	fresh := runtime.Delivery().Providers()
	assert.Equal(t, []string{"payments"}, fresh[0].Routes[0].Owners)
}
