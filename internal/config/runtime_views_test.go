package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntimeConfigGroupedViewsAreDefensive(t *testing.T) {
	config := &Config{
		AllowedNamespaces: []string{"team-a"},
		Alert: map[string]map[string]interface{}{
			"webhook": {
				"routes": []interface{}{map[string]interface{}{
					"namespaces": []interface{}{"team-a"},
				}},
			},
		},
	}
	runtime := CompileRuntimeConfig(config)

	scope := runtime.Scope()
	namespaces := scope.AllowedNamespaces()
	namespaces[0] = "mutated"
	require.Equal(t, []string{"team-a"},
		runtime.Scope().AllowedNamespaces())

	delivery := runtime.Delivery()
	providers := delivery.Providers()
	routes := providers[0].Settings["routes"].([]interface{})
	route := routes[0].(map[string]interface{})
	names := route["namespaces"].([]interface{})
	names[0] = "mutated"
	freshRoutes := runtime.Delivery().Providers()[0].
		Settings["routes"].([]interface{})
	freshRoute := freshRoutes[0].(map[string]interface{})
	require.Equal(t, "team-a",
		freshRoute["namespaces"].([]interface{})[0])

	monitor := runtime.Monitors()
	require.Equal(t, runtime.NodeMonitor(), monitor.Node())
}
