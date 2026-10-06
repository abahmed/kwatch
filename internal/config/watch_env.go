package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// watchSecretsEnv lets the Helm chart turn Secret watching off even when
// the operator supplies config.yaml in their own Secret, so the
// configuration always matches the RBAC the chart rendered. It can only
// turn watching off, never on.
const watchSecretsEnv = "KWATCH_WATCH_SECRETS"

// crdEnabledEnv carries the chart's config.crd.enabled value. The chart
// installs the KwatchConfig CRD and its RBAC, and config.yaml may come from
// an operator Secret that never mentions crd, so the chart value wins in
// both directions.
const crdEnabledEnv = "KWATCH_CRD_ENABLED"

// telemetryEnv is checked only for syntax here: internal/app reads it to
// decide whether to send the heartbeat.
const telemetryEnv = "KWATCH_TELEMETRY"

// ParseEnvBool reads a boolean environment value. It accepts what
// strconv.ParseBool accepts plus on/off and yes/no, in any letter case.
// set is false for an empty (or all-space) value; err is set for any
// other text, so a typo is reported instead of silently ignored.
func ParseEnvBool(name, raw string) (value, set bool, err error) {
	text := strings.ToLower(strings.TrimSpace(raw))
	switch text {
	case "":
		return false, false, nil
	case "on", "yes":
		return true, true, nil
	case "off", "no":
		return false, true, nil
	}
	parsed, err := strconv.ParseBool(text)
	if err != nil {
		return false, false, fmt.Errorf(
			"%s=%q is not a boolean: use true/false, on/off, yes/no or 1/0",
			name, raw)
	}
	return parsed, true, nil
}

// applyEnvironmentOverrides applies the deployment environment. It runs
// after config.yaml and again after a KwatchConfig overlay, so an overlay
// can never turn on something the deployment turned off. An invalid
// boolean is an error, so a typo fails startup.
func applyEnvironmentOverrides(c *Config) error {
	var errs []error
	errs = append(errs, applyWatchEnvironment(c))
	errs = append(errs, applyCRDEnvironment(c))
	_, _, err := ParseEnvBool(telemetryEnv, os.Getenv(telemetryEnv))
	errs = append(errs, err)
	return errors.Join(errs...)
}

// applyWatchEnvironment applies KWATCH_WATCH_SECRETS=false. A true value
// changes nothing: the environment can only turn watching off.
func applyWatchEnvironment(c *Config) error {
	value, set, err := ParseEnvBool(
		watchSecretsEnv, os.Getenv(watchSecretsEnv))
	if set && !value {
		c.Watch.Secrets = false
	}
	return err
}

// applyCRDEnvironment applies KWATCH_CRD_ENABLED. Unset keeps config.yaml's
// setting.
func applyCRDEnvironment(c *Config) error {
	value, set, err := ParseEnvBool(crdEnabledEnv, os.Getenv(crdEnabledEnv))
	if set {
		c.CrdConfig.Enabled = value
	}
	return err
}
