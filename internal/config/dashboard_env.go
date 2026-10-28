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
	dashboardModeEnv    = "KWATCH_DASHBOARD_MODE"
	dashboardTrustEnv   = "KWATCH_DASHBOARD_TRUST_PROXY"
	dashboardCertEnv    = "KWATCH_DASHBOARD_TLS_CERT"
	dashboardKeyEnv     = "KWATCH_DASHBOARD_TLS_KEY"
)

// applyDashboardEnv applies the dashboard port and mode. A value that
// does not parse is a warning and the file's setting stays; validation
// still rejects an unknown mode written in the file.
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
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(dashboardModeEnv)))
	switch raw {
	case "":
	case DashboardLocal, DashboardShared:
		c.Dashboard.Mode = raw
	default:
		c.envWarnings = append(c.envWarnings, fmt.Sprintf(
			"%s=%q is not local or shared; the file's dashboard.mode "+
				"stays", dashboardModeEnv, raw))
	}
}
