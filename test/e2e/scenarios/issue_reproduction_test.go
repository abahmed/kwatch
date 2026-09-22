//go:build e2e

package scenarios

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioImportedIssue(t *testing.T) {
	fixture := os.Getenv("ISSUE_FIXTURE_DIR")
	if fixture == "" {
		t.Skip("ISSUE_FIXTURE_DIR is not configured")
	}
	runScenario(t, "issue.imported-reproduction", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		resources := filepath.Join(fixture, "resources.yaml")
		if err := e.ApplyFixture(ctx, resources); err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := e.AssertHealthy(waitCtx); err != nil {
			t.Fatal(err)
		}
		if err := e.AssertNoRuntimePanic(waitCtx); err != nil {
			t.Fatal(err)
		}
		if reason := os.Getenv("ISSUE_EXPECTED_REASON"); reason != "" {
			if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
				Namespace: valueOrDefault("ISSUE_NAMESPACE", "kwatch-issue"),
				Reason:    reason, Action: "create", Count: 1,
			}); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func valueOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
