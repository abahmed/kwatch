package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// The chart's dashboard values reach kwatch through the environment, so
// they hold even when config.yaml comes from an operator Secret that
// never mentions the dashboard. They win over the file in both
// directions, like crd.enabled.
const (
	dashboardEnabledEnv = "KWATCH_DASHBOARD_ENABLED"
	dashboardPortEnv    = "KWATCH_DASHBOARD_PORT"
	dashboardProxiesEnv = "KWATCH_DASHBOARD_TRUSTED_PROXIES"
	dashboardCertEnv    = "KWATCH_DASHBOARD_TLS_CERT"
	dashboardKeyEnv     = "KWATCH_DASHBOARD_TLS_KEY"
)

// applyDashboardEnv applies the dashboard port, TLS files and trusted
// proxies. A port that does not parse is a warning and the file's
// setting stays. The proxy list wins over the file in both directions.
func applyDashboardEnv(c *Config) {
	if raw := strings.TrimSpace(os.Getenv(dashboardPortEnv)); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil {
			c.envWarnings = append(c.envWarnings, fmt.Sprintf(
				"%s=%q is not a port; the file's dashboard.port stays",
				dashboardPortEnv, raw))
		} else {
			c.Dashboard.Port = port
		}
	}
	if v := strings.TrimSpace(os.Getenv(dashboardCertEnv)); v != "" {
		c.Dashboard.TLSCertFile = v
	}
	if v := strings.TrimSpace(os.Getenv(dashboardKeyEnv)); v != "" {
		c.Dashboard.TLSKeyFile = v
	}
	if raw, ok := os.LookupEnv(dashboardProxiesEnv); ok {
		c.Dashboard.TrustedProxies = splitList(raw)
	}
}

// splitList splits a comma-separated list, dropping blanks.
func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
