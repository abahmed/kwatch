package app

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

func TestDashboardDetailKeepsStepsWithoutTheRunbook(t *testing.T) {
	const runbook = "https://runbooks.example.com/crash"
	const logs = "kubectl -n shop logs deploy/web"
	m := notification.Message{Title: "web is crash looping",
		Steps: []notification.Step{
			{Text: "Runbook for CrashLoop: " + runbook},
			{Text: "Read the logs:", Command: logs},
			{Text: "Rotate it", Command: "curl -H " +
				"'Authorization: Bearer abcdefghijklmnop1234'"},
		}}
	d := dashboardDetail(incident.Decision{}, m)
	if d.Headline != m.Title || d.DocsLink != runbook {
		t.Fatalf("detail = %+v", d)
	}
	if len(d.Steps) != 2 || d.Steps[0].Command != logs {
		t.Fatalf("steps = %+v", d.Steps)
	}
	if d.Steps[1].Command == m.Steps[2].Command {
		t.Fatalf("a secret in a command was kept: %q", d.Steps[1].Command)
	}
}

func TestNewDashboardServerNeedsProAndEnabled(t *testing.T) {
	now := clock.Func(func() time.Time { return time.Unix(0, 0) })
	cfg := config.DefaultConfig()
	pro := feature.ProEntitlements()
	if newDashboardServer(config.CompileRuntimeConfig(cfg), now, pro) != nil {
		t.Fatal("a disabled dashboard was built")
	}
	cfg.Dashboard.Enabled = true
	cfg.Dashboard.Mode = config.DashboardShared
	runtime := config.CompileRuntimeConfig(cfg)
	if newDashboardServer(runtime, now, nil) != nil {
		t.Fatal("Community built the dashboard")
	}
	t.Setenv(dashboardPasswordEnv, "pw")
	t.Setenv(dashboardDeploymentEnv, "kwatch-prod")
	if newDashboardServer(runtime, now, pro) == nil {
		t.Fatal("Pro with the dashboard enabled built nothing")
	}
}
