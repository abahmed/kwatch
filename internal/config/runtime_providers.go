package config

import (
	"fmt"
	"strings"
	"time"
)

const runtimeMaxRetryAttempts = 20

// RetryPolicy is the normalized retry policy used by delivery. Keeping it in
// the runtime snapshot means raw provider maps are parsed once.
type RetryPolicy struct {
	MaxAttempts   int
	Delay         time.Duration
	MaxBackoff    time.Duration
	JitterEnabled bool
	JitterFactor  float64
}

// ProviderRuntime is the immutable provider view consumed by composition.
// Settings remain map-shaped because provider-specific configuration is
// intentionally extensible; returned values are defensive copies.
type ProviderRuntime struct {
	Name         string
	Settings     map[string]interface{}
	Templates    map[string]string
	Routes       []AlertRoute
	Retry        RetryPolicy
	FallbackName string
}

func compileProviderRuntimes(
	alert map[string]map[string]interface{}, names []string,
) []ProviderRuntime {
	result := make([]ProviderRuntime, 0, len(names))
	for _, name := range names {
		settings := cloneRuntimeMap(alert[name])
		result = append(result, ProviderRuntime{
			Name:         name,
			Settings:     settings,
			Templates:    compileProviderTemplates(settings),
			Routes:       compileRoutes(settings),
			Retry:        compileRetry(settings),
			FallbackName: stringValue(settings["fallback"]),
		})
	}
	return result
}

func compileProviderTemplates(
	settings map[string]interface{},
) map[string]string {
	raw, ok := settings["templates"]
	if !ok {
		return nil
	}
	result := make(map[string]string)
	switch templates := raw.(type) {
	case map[string]interface{}:
		for reason, value := range templates {
			if body, ok := value.(string); ok {
				result[reason] = body
			}
		}
	case map[string]string:
		for reason, body := range templates {
			result[reason] = body
		}
	default:
		return nil
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func compileRoutes(settings map[string]interface{}) []AlertRoute {
	rawRoutes, ok := settings["routes"].([]interface{})
	if !ok {
		return nil
	}
	routes := make([]AlertRoute, 0, len(rawRoutes))
	for _, raw := range rawRoutes {
		routeMap, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		route := AlertRoute{
			Namespaces: runtimeStringList(routeMap["namespaces"]),
			Severities: runtimeStringList(routeMap["severities"]),
			Reasons:    runtimeStringList(routeMap["reasons"]),
		}
		if len(route.Namespaces) > 0 || len(route.Severities) > 0 ||
			len(route.Reasons) > 0 {
			routes = append(routes, route)
		}
	}
	return routes
}

func runtimeStringList(value interface{}) []string {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, fmt.Sprint(item))
	}
	return values
}

func compileRetry(settings map[string]interface{}) RetryPolicy {
	policy := RetryPolicy{
		MaxAttempts: 3, Delay: time.Second,
		MaxBackoff: 30 * time.Second, JitterFactor: 0.25,
	}
	values, ok := settings["retry"].(map[string]interface{})
	if !ok {
		return policy
	}
	if attempts, ok := runtimeInt(values["maxAttempts"]); ok {
		if attempts > runtimeMaxRetryAttempts {
			attempts = runtimeMaxRetryAttempts
		}
		if attempts < 1 {
			attempts = 1
		}
		policy.MaxAttempts = attempts
	}
	if delay, ok := runtimeDuration(values["delay"]); ok {
		policy.Delay = delay
	}
	if maxBackoff, ok := runtimeDuration(values["maxBackoff"]); ok {
		policy.MaxBackoff = maxBackoff
	}
	if enabled, ok := values["jitterEnabled"].(bool); ok {
		policy.JitterEnabled = enabled
	}
	if factor, ok := runtimeFloat(values["jitterFactor"]); ok {
		policy.JitterFactor = factor
	}
	if policy.JitterFactor < 0 {
		policy.JitterFactor = 0
	}
	if policy.JitterFactor > 1 {
		policy.JitterFactor = 1
	}
	if policy.Delay <= 0 {
		policy.Delay = time.Second
	}
	if policy.MaxBackoff < 0 {
		policy.MaxBackoff = 30 * time.Second
	}
	return policy
}

func runtimeInt(value interface{}) (int, bool) {
	switch value := value.(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	default:
		return 0, false
	}
}

func runtimeFloat(value interface{}) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case int:
		return float64(value), true
	default:
		return 0, false
	}
}

func runtimeDuration(value interface{}) (time.Duration, bool) {
	text, ok := value.(string)
	if !ok {
		return 0, false
	}
	duration, err := time.ParseDuration(strings.TrimSpace(text))
	return duration, err == nil
}

func stringValue(value interface{}) string {
	text, _ := value.(string)
	return text
}

func cloneRuntimeMap(values map[string]interface{}) map[string]interface{} {
	if values == nil {
		return nil
	}
	result := make(map[string]interface{}, len(values))
	for key, value := range values {
		result[key] = cloneRuntimeValue(value)
	}
	return result
}

func cloneRuntimeValue(value interface{}) interface{} {
	switch value := value.(type) {
	case map[string]interface{}:
		return cloneRuntimeMap(value)
	case map[string]string:
		result := make(map[string]string, len(value))
		for key, item := range value {
			result[key] = item
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(value))
		for i, item := range value {
			result[i] = cloneRuntimeValue(item)
		}
		return result
	case []string:
		return cloneStrings(value)
	default:
		return value
	}
}

func cloneProviderRuntimes(
	providers []ProviderRuntime,
) []ProviderRuntime {
	result := make([]ProviderRuntime, 0, len(providers))
	for _, provider := range providers {
		result = append(result, ProviderRuntime{
			Name:         provider.Name,
			Settings:     cloneRuntimeMap(provider.Settings),
			Templates:    cloneStringMap(provider.Templates),
			Routes:       cloneRoutes(provider.Routes),
			Retry:        provider.Retry,
			FallbackName: provider.FallbackName,
		})
	}
	return result
}

func cloneRoutes(routes []AlertRoute) []AlertRoute {
	result := make([]AlertRoute, 0, len(routes))
	for _, route := range routes {
		result = append(result, AlertRoute{
			Namespaces: cloneStrings(route.Namespaces),
			Severities: cloneStrings(route.Severities),
			Reasons:    cloneStrings(route.Reasons),
		})
	}
	return result
}
