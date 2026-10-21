package config

import (
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

// telemetryEnv turns adoption telemetry off; internal/app also reads it
// when it decides whether to send the heartbeat.
const telemetryEnv = "KWATCH_TELEMETRY"

// licenseKeyEnv carries the license key, typically from a Secret. It wins
// over license.key in the file.
const licenseKeyEnv = "KWATCH_LICENSE_KEY"

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

// clusterCheckinsEnv turns the Pro cluster check-ins off. Like
// KWATCH_TELEMETRY it can only opt out: a true value changes nothing.
const clusterCheckinsEnv = "KWATCH_CLUSTER_CHECKINS"

// applyEnvironmentOverrides applies the deployment environment. It runs
// after config.yaml and again after a KwatchConfig overlay, so an overlay
// can never turn on something the deployment turned off.
//
// A boolean that does not parse is a warning, never a startup failure: a
// typo in an environment variable must not stop monitoring. The value is
// ignored, and it is never read as "on": KWATCH_TELEMETRY and
// KWATCH_CLUSTER_CHECKINS switch off, KWATCH_WATCH_SECRETS stops the Secret
// watch, and KWATCH_CRD_ENABLED keeps the file's setting.
func applyEnvironmentOverrides(c *Config) {
	c.envWarnings = nil
	for _, rule := range envRules {
		value, set, err := ParseEnvBool(rule.name, os.Getenv(rule.name))
		if err != nil {
			c.envWarnings = append(c.envWarnings, fmt.Sprintf(
				"%s=%q is not a boolean (use true/false, on/off, "+
					"yes/no or 1/0); it is ignored and %s",
				rule.name, os.Getenv(rule.name), rule.onInvalid))
			if rule.invalid != nil {
				rule.invalid(c)
			}
			continue
		}
		if set {
			rule.apply(c, value)
		}
	}
	if key := os.Getenv(licenseKeyEnv); strings.TrimSpace(key) != "" {
		c.License.Key = key
	}
}

// envRule is one boolean environment variable: what a valid value does
// and what an invalid one falls back to.
type envRule struct {
	name      string
	apply     func(c *Config, value bool)
	invalid   func(c *Config)
	onInvalid string
}

var envRules = []envRule{
	{
		name: watchSecretsEnv,
		// The environment can only turn Secret watching off.
		apply: func(c *Config, value bool) {
			if !value {
				c.Watch.Secrets = false
			}
		},
		invalid:   func(c *Config) { c.Watch.Secrets = false },
		onInvalid: "kwatch does not watch Secrets",
	},
	{
		name:      crdEnabledEnv,
		apply:     func(c *Config, value bool) { c.CrdConfig.Enabled = value },
		onInvalid: "the file's crd.enabled setting stays",
	},
	{
		name: telemetryEnv,
		apply: func(c *Config, value bool) {
			if !value {
				c.Telemetry.Enabled = false
			}
		},
		invalid:   func(c *Config) { c.Telemetry.Enabled = false },
		onInvalid: "adoption telemetry is off",
	},
	{
		name: clusterCheckinsEnv,
		apply: func(c *Config, value bool) {
			if !value {
				c.ClusterCheckins = false
			}
		},
		invalid:   func(c *Config) { c.ClusterCheckins = false },
		onInvalid: "cluster check-ins are off",
	},
}
