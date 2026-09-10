package config

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/labels"
)

func validateSelectors(cfg *Config) []error {
	if cfg.NamespaceSelector == "" {
		return nil
	}
	if _, err := labels.Parse(cfg.NamespaceSelector); err != nil {
		return []error{fmt.Errorf(
			"namespaceSelector is not a valid Kubernetes label selector: %w",
			err,
		)}
	}
	return nil
}

func validateAlertRetries(cfg *Config) []error {
	names := make([]string, 0, len(cfg.Alert))
	for name := range cfg.Alert {
		names = append(names, name)
	}
	sort.Strings(names)

	var errs []error
	for _, name := range names {
		raw, ok := cfg.Alert[name]["retry"]
		if !ok {
			continue
		}
		rm, ok := raw.(map[string]interface{})
		if !ok {
			errs = append(errs, fmt.Errorf(
				"alert.%s.retry must be a mapping", name,
			))
			continue
		}
		errs = append(errs, validateRetryMaxAttempts(name, rm)...)
		errs = append(errs, validateRetryDuration(name, rm, "delay", false)...)
		errs = append(errs, validateRetryDuration(name, rm, "maxBackoff", true)...)
		errs = append(errs, validateRetryJitterFields(name, rm)...)
	}
	return errs
}

func validateRetryMaxAttempts(
	provider string,
	rm map[string]interface{},
) []error {
	raw, ok := rm["maxAttempts"]
	if !ok {
		return nil
	}
	n, ok := numericValue(raw)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) ||
		math.Trunc(n) != n || n < 1 || n > 20 {
		return []error{fmt.Errorf(
			"alert.%s.retry.maxAttempts must be an integer between 1 and 20",
			provider,
		)}
	}
	return nil
}

func validateRetryDuration(
	provider string,
	rm map[string]interface{},
	key string,
	allowZero bool,
) []error {
	raw, ok := rm[key]
	if !ok {
		return nil
	}
	s, ok := raw.(string)
	if !ok {
		return []error{fmt.Errorf(
			"alert.%s.retry.%s must be a duration string",
			provider,
			key,
		)}
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 || (!allowZero && d == 0) {
		bound := "> 0"
		if allowZero {
			bound = ">= 0"
		}
		return []error{fmt.Errorf(
			"alert.%s.retry.%s must be a valid duration %s",
			provider,
			key,
			bound,
		)}
	}
	return nil
}

func validateRetryJitterFields(
	provider string,
	rm map[string]interface{},
) []error {
	var errs []error
	if raw, ok := rm["jitterEnabled"]; ok {
		if _, valid := raw.(bool); !valid {
			errs = append(errs, fmt.Errorf(
				"alert.%s.retry.jitterEnabled must be boolean",
				provider,
			))
		}
	}
	if raw, ok := rm["jitterFactor"]; ok {
		factor, valid := numericValue(raw)
		if !valid || math.IsNaN(factor) || math.IsInf(factor, 0) ||
			factor < 0 || factor > 1 {
			errs = append(errs, fmt.Errorf(
				"alert.%s.retry.jitterFactor must be a number between 0 and 1",
				provider,
			))
		}
	}
	return errs
}

func numericValue(raw interface{}) (float64, bool) {
	switch value := raw.(type) {
	case int:
		return float64(value), true
	case int8:
		return float64(value), true
	case int16:
		return float64(value), true
	case int32:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint:
		return float64(value), true
	case uint8:
		return float64(value), true
	case uint16:
		return float64(value), true
	case uint32:
		return float64(value), true
	case uint64:
		return float64(value), true
	case float32:
		return float64(value), true
	case float64:
		return value, true
	case json.Number:
		number, err := value.Float64()
		return number, err == nil
	default:
		return 0, false
	}
}
