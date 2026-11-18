package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// logMissingScenarios are workloads whose pods stay Ready while their
// log keeps failing to resolve a Service-shaped name, and the cases
// that must stay quiet.
func logMissingScenarios() []scenario {
	return []scenario{logMissingService(), logMissingAtStartOnly(),
		logMissingServiceExists()}
}

// logMissingService: reports looks up Service paymnts-api, which does
// not exist (payments-api does), on every sampled window for forty
// minutes. One low finding on reports, in the digest.
func logMissingService() scenario {
	return scenario{
		expect: expectation{
			Name: "log-missing-service",
			Description: "A Ready workload's log keeps failing to " +
				"resolve a Service that does not exist, for forty " +
				"minutes.",
			Root: "deployment/shop/reports", Tier: "digest",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n1"},
			Tail:         duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			buildLogMissing(c, "paymnts-api", 20, 20)
		},
	}
}

// logMissingAtStartOnly: the same lookup fails once, at start-up, and
// never again. One miss never counts.
func logMissingAtStartOnly() scenario {
	return scenario{
		expect: expectation{
			Name: "log-missing-at-start-only",
			Description: "A Ready workload's log fails to resolve a " +
				"Service once, at start-up, and never again.",
			Quiet: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			buildLogMissing(c, "paymnts-api", 20, 1)
		},
	}
}

// logMissingServiceExists: the name the log fails on is a Service that
// exists (its lookups fail for another reason). Nothing is said.
func logMissingServiceExists() scenario {
	return scenario{
		expect: expectation{
			Name: "log-missing-service-exists",
			Description: "A Ready workload's log keeps failing to " +
				"resolve a Service that exists.",
			Quiet: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			buildLogMissing(c, "payments-api", 20, 20)
		},
	}
}

// buildLogMissing samples reports' log windows times, two minutes
// apart; the first failing windows hold the failed lookup of name.
func buildLogMissing(c *cluster, name string, windows, failing int) {
	c.list(c.node("n1", "zone-a"),
		&corev1.Namespace{ObjectMeta: c.meta("", "shop")},
		clusterService(c, "shop", "payments-api", 80))
	app := c.deployment("shop", "reports", "registry.example.com/r:2", 2)
	app.setReady(2)
	c.list(app.objects())
	c.list(app.pod(0, "n1"), app.pod(1, "n1"))
	id := inventory.CoreID(kube.KindDeployment, "shop",
		app.deployment.Name)
	line := "warn: request failed: Name or service not known (" +
		c.n(name) + ":80)"
	var sampled [][]string
	for i := range windows {
		lines := []string{"info: report built in 40ms"}
		if i < failing {
			lines = append(lines, line)
		}
		sampled = append(sampled, lines)
		c.emit(kube.LogSampleObservation(id, c.now, sampled))
		c.after(2 * time.Minute)
	}
}
