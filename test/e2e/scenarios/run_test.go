//go:build e2e

package scenarios

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/abahmed/kwatch/test/e2e/coverage"
	"github.com/abahmed/kwatch/test/e2e/harness"
)

// This file is the plumbing under inNamespace and onCluster: skipping,
// sharding, namespaces and diagnostics. Scenarios never call it directly.

// ptr returns a pointer to value, for optional Kubernetes fields.
func ptr[T any](value T) *T {
	return &value
}

// workloadImage is the local image that crashes, sleeps or serves on demand.
func workloadImage() string {
	if value := os.Getenv("WORKLOAD_IMAGE"); value != "" {
		return value
	}
	return "kwatch-e2e-workload:e2e"
}

// runScenario runs one scenario through the e2e framework, unless the suite
// is not enabled or the scenario belongs to another shard.
func runScenario(
	t *testing.T,
	id string,
	run func(context.Context, *testing.T, *harness.Environment),
) {
	t.Helper()
	runScenarioAfter(t, id, waitForColdStart, run)
}

// runScenarioAfter is runScenario with its own rule for how long to wait
// for Kwatch before the scenario starts.
func runScenarioAfter(
	t *testing.T,
	id string,
	waitForKwatch func(context.Context, *harness.Environment) error,
	run func(context.Context, *testing.T, *harness.Environment),
) {
	t.Helper()
	if os.Getenv("KWATCH_E2E") != "true" {
		t.Skip("set KWATCH_E2E=true to run real-cluster scenarios")
	}
	if !belongsToShard(id, os.Getenv("SCENARIO_SHARD")) {
		t.Skip("scenario belongs to another shard")
	}
	config, err := harness.ConfigFromEnvChecked()
	if err != nil {
		t.Fatal(err)
	}
	config.Artifacts = config.Artifacts + "/" + strings.NewReplacer(
		"/", "_", " ", "_",
	).Replace(id)
	frameworkEnvironment := env.NewWithKubeConfig(config.Kubeconfig)
	feature := features.New(id).Assess("scenario", func(
		ctx context.Context,
		t *testing.T,
		_ *envconf.Config,
	) context.Context {
		environment, err := harness.NewEnvironment(config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = environment.Close() })
		if err := waitForKwatch(ctx, environment); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { captureDiagnosticsIfFailed(t, environment) })
		scenarioCtx, cancel := context.WithTimeout(
			ctx, config.ScenarioTimeout,
		)
		defer cancel()
		run(scenarioCtx, t, environment)
		return ctx
	}).Feature()
	frameworkEnvironment.Test(t, feature)
}

// captureDiagnosticsIfFailed saves cluster state and Kwatch logs as test
// artifacts when the test has failed.
func captureDiagnosticsIfFailed(t *testing.T, e *harness.Environment) {
	t.Helper()
	if !t.Failed() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := e.CaptureDiagnosticsOnce(ctx); err != nil {
		t.Errorf("capture diagnostics: %v", err)
	}
}

var namespaceSequence atomic.Uint32

// uniqueNamespace names a scenario namespace so that parallel runs and
// repeated scenarios never collide.
func uniqueNamespace(name string) string {
	value := os.Getenv("GITHUB_RUN_ID")
	if value == "" {
		value = time.Now().Format("20060102150405")
	}
	sequence := namespaceSequence.Add(1)
	hash := sha1Sum(name + ":" + value)
	suffix := hex.EncodeToString(hash[:])[:8]
	return "kwatch-e2e-" + value + "-" + suffix +
		"-" + strconv.FormatUint(uint64(sequence), 10)
}

func createNamespace(
	ctx context.Context, e *harness.Environment, name string,
) error {
	_, err := e.Client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}, metav1.CreateOptions{})
	return err
}

// cleanupNamespace deletes the scenario namespace and waits until it is
// gone. A failed test keeps its diagnostics first.
func cleanupNamespace(t *testing.T, e *harness.Environment, name string) {
	t.Helper()
	captureDiagnosticsIfFailed(t, e)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := e.DeleteNamespace(ctx, name); err != nil {
		t.Errorf("delete scenario namespace: %v", err)
	}
}

func sha1Sum(text string) [sha1.Size]byte {
	return sha1.Sum([]byte(text))
}

// belongsToShard reports whether this run owns the scenario. value is
// "index/total" (for example "2/4"); an empty value owns every scenario.
func belongsToShard(id, value string) bool {
	if value == "" {
		return true
	}
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return false
	}
	index, indexErr := strconv.Atoi(parts[0])
	total, totalErr := strconv.Atoi(parts[1])
	if indexErr != nil || totalErr != nil || index < 1 || index > total {
		return false
	}
	return shardOf(id, total) == index-1
}

// scenarioTests maps every covered scenario ID to its test function.
func scenarioTests() map[string]string {
	tests := map[string]string{}
	for _, entry := range coveredEntries() {
		tests[entry.ID] = entry.Test
	}
	return tests
}

var coveredEntries = sync.OnceValue(func() []coverage.Entry {
	path := filepath.Join("..", "coverage", "coverage.yaml")
	catalog, err := coverage.Load(path)
	if err != nil {
		return nil
	}
	var entries []coverage.Entry
	for _, entry := range catalog.Entries {
		if entry.Status == "covered" && entry.Test != "" {
			entries = append(entries, entry)
		}
	}
	return entries
})
