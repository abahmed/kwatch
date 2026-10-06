//go:build e2e

package harness

import (
	"fmt"
	"os"
	"os/exec"
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
}

// allowAnyContextEnv opts in to running against a cluster whose kubectl
// context is not a kind context. The suite creates and deletes namespaces
// and workloads, so it must not hit a real cluster by accident.
const allowAnyContextEnv = "KWATCH_E2E_ALLOW_ANY_CONTEXT"

// ConfigFromEnv is ConfigFromEnvChecked for callers that cannot return an
// error: it panics when the kubectl context is not allowed.
func ConfigFromEnv() Config {
	config, err := ConfigFromEnvChecked()
	if err != nil {
		panic(err)
	}
	return config
}

// ConfigFromEnvChecked reads the environment and refuses a kubectl context
// that is not a kind cluster (name starting "kind-") unless
// KWATCH_E2E_ALLOW_ANY_CONTEXT=true. An empty KUBE_CONTEXT means kubectl's
// current context, which is resolved and checked the same way.
func ConfigFromEnvChecked() (Config, error) {
	config := configFromEnv()
	name := config.Context
	if name == "" {
		current, err := currentContext(config.Kubeconfig)
		if err != nil {
			return Config{}, fmt.Errorf(
				"resolve kubectl context (set KUBE_CONTEXT): %w", err)
		}
		name = current
	}
	allow := os.Getenv(allowAnyContextEnv) == "true"
	if err := checkContext(name, allow); err != nil {
		return Config{}, err
	}
	return config, nil
}

// checkContext allows kind contexts, and any other only on explicit opt-in.
func checkContext(name string, allowAny bool) error {
	if strings.HasPrefix(name, "kind-") || allowAny {
		return nil
	}
	return fmt.Errorf(
		"kubectl context %q is not a kind cluster: refusing to run the "+
			"e2e suite; use a kind-* context or set %s=true",
		name, allowAnyContextEnv)
}

// currentContext asks kubectl for its current context name.
func currentContext(kubeconfig string) (string, error) {
	args := []string{"config", "current-context"}
	if kubeconfig != "" {
		args = append([]string{"--kubeconfig", kubeconfig}, args...)
	}
	out, err := exec.Command("kubectl", args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func configFromEnv() Config {
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
		Artifacts:       valueOr("ARTIFACTS", "artifacts"),
		KwatchImage:     valueOr("KWATCH_IMAGE", "kwatch:e2e"),
		ScenarioTimeout: durationOr("SCENARIO_TIMEOUT", 10*time.Minute),
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
