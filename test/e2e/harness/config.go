//go:build e2e

package harness

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Kubeconfig        string
	Context           string
	KwatchNamespace   string
	ReceiverNamespace string
	ReceiverService   string
	Artifacts         string
	KwatchImage       string
	ScenarioTimeout   time.Duration
	SuiteTimeout      time.Duration
	DiagnosticsToken  string
}

func ConfigFromEnv() Config {
	return Config{
		Kubeconfig:      os.Getenv("KUBECONFIG"),
		Context:         os.Getenv("KUBE_CONTEXT"),
		KwatchNamespace: valueOr("KWATCH_NAMESPACE", "kwatch"),
		ReceiverNamespace: valueOr(
			"KWATCH_RECEIVER_NAMESPACE", "kwatch-e2e-system",
		),
		ReceiverService: valueOr(
			"KWATCH_RECEIVER_SERVICE", "kwatch-e2e-receiver",
		),
		Artifacts:        valueOr("ARTIFACTS", "artifacts"),
		KwatchImage:      valueOr("KWATCH_IMAGE", "kwatch:e2e"),
		ScenarioTimeout:  durationOr("SCENARIO_TIMEOUT", 10*time.Minute),
		SuiteTimeout:     durationOr("SUITE_TIMEOUT", 60*time.Minute),
		DiagnosticsToken: valueOr("KWATCH_DIAGNOSTICS_TOKEN", "e2e-token"),
	}
}

func valueOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func durationOr(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	if duration, err := time.ParseDuration(value); err == nil {
		return duration
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}
