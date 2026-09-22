//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"

	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func runScenario(
	t *testing.T,
	id string,
	run func(context.Context, *testing.T, *harness.Environment),
) {
	t.Helper()
	if os.Getenv("KWATCH_E2E") != "true" {
		t.Skip("set KWATCH_E2E=true to run real-cluster scenarios")
	}
	config := harness.ConfigFromEnv()
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
		run(ctx, t, environment)
		return ctx
	}).Feature()
	frameworkEnvironment.Test(t, feature)
}
