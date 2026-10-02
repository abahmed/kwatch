package config

import (
	"os"
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

// applyEnvironmentOverrides applies the deployment environment. It runs
// after config.yaml and again after a KwatchConfig overlay, so an overlay
// can never turn on something the deployment turned off.
func applyEnvironmentOverrides(c *Config) {
	applyWatchEnvironment(c)
	applyCRDEnvironment(c)
}

// applyWatchEnvironment applies KWATCH_WATCH_SECRETS=false.
func applyWatchEnvironment(c *Config) {
	value := strings.TrimSpace(os.Getenv(watchSecretsEnv))
	if strings.EqualFold(value, "false") {
		c.Watch.Secrets = false
	}
}

// applyCRDEnvironment applies KWATCH_CRD_ENABLED=true or false. Any other
// value, including unset, keeps config.yaml's setting.
func applyCRDEnvironment(c *Config) {
	value := strings.TrimSpace(os.Getenv(crdEnabledEnv))
	switch {
	case strings.EqualFold(value, "true"):
		c.CrdConfig.Enabled = true
	case strings.EqualFold(value, "false"):
		c.CrdConfig.Enabled = false
	}
}
