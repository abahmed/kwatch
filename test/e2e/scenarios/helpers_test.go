//go:build e2e

package scenarios

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func createNamespace(
	ctx context.Context,
	e *harness.Environment,
	name string,
) error {
	_, err := e.Client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}, metav1.CreateOptions{})
	return err
}

func cleanupNamespace(t *testing.T, e *harness.Environment, name string) {
	t.Helper()
	diagnosticsCtx, diagnosticsCancel := context.WithTimeout(
		context.Background(), 45*time.Second,
	)
	defer diagnosticsCancel()
	if err := e.CaptureDiagnostics(diagnosticsCtx); err != nil {
		t.Errorf("capture diagnostics: %v", err)
	}
	cleanupCtx, cancel := context.WithTimeout(
		context.Background(), 2*time.Minute,
	)
	defer cancel()
	if err := e.DeleteNamespace(cleanupCtx, name); err != nil {
		t.Errorf("delete scenario namespace: %v", err)
	}
}

func runScenario(
	t *testing.T,
	id string,
	run func(context.Context, *testing.T, *harness.Environment),
) {
	t.Helper()
	if os.Getenv("KWATCH_E2E") != "true" {
		t.Skip("set KWATCH_E2E=true to run real-cluster scenarios")
	}
	if !belongsToShard(id, os.Getenv("SCENARIO_SHARD")) {
		t.Skip("scenario belongs to another shard")
	}
	config := harness.ConfigFromEnv()
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
		t.Cleanup(func() {
			captureCtx, cancel := context.WithTimeout(
				context.Background(), 45*time.Second,
			)
			defer cancel()
			if err := environment.CaptureDiagnostics(captureCtx); err != nil {
				t.Errorf("capture diagnostics: %v", err)
			}
		})
		run(ctx, t, environment)
		return ctx
	}).Feature()
	frameworkEnvironment.Test(t, feature)
}

func runExtendedScenario(
	t *testing.T,
	id string,
	run func(context.Context, *testing.T, *harness.Environment),
) {
	t.Helper()
	if os.Getenv("KWATCH_EXTENDED") != "true" {
		t.Skip("set KWATCH_EXTENDED=true for extended Kind scenarios")
	}
	runScenario(t, id, run)
}

func uniqueNamespace(name string) string {
	value := os.Getenv("GITHUB_RUN_ID")
	if value == "" {
		value = time.Now().Format("20060102150405")
	}
	sequence := atomic.AddUint32(&namespaceSequence, 1)
	hash := sha1.Sum([]byte(name + ":" + value))
	suffix := hex.EncodeToString(hash[:])[:8]
	return "kwatch-e2e-" + value + "-" + suffix +
		"-" + namespaceSequenceString(sequence)
}

var namespaceSequence uint32

func namespaceSequenceString(sequence uint32) string {
	return strconv.FormatUint(uint64(sequence), 10)
}

func workloadImage() string {
	if value := os.Getenv("WORKLOAD_IMAGE"); value != "" {
		return value
	}
	return "kwatch-e2e-workload:e2e"
}

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
	hash := sha1.Sum([]byte(id))
	return int(hash[0])%total == index-1
}
