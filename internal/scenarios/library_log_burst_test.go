package scenarios

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// logBurstScenarios are workloads whose pods are Running and Ready while
// their log reports errors. Kubernetes has nothing to say; kwatch judges
// the last sample window against the workload's own earlier windows.
func logBurstScenarios() []scenario {
	return []scenario{
		healthyPodsErrorBurst(), healthyPodsSteadyErrors(),
		healthyPodsSmallBurst(), healthyPodsBurstWithoutBaseline(),
	}
}

// errorWindow is one sample window of the orders log: n repeats of the
// line, with ordinary info lines around them.
func errorWindow(n int, line string) []string {
	lines := []string{"info request served in 12ms"}
	for i := range n {
		lines = append(lines, fmt.Sprintf("%s id=%d", line, 1000+i))
	}
	return append(lines, "info request served in 9ms")
}

const ordersRefused = "level=error msg=\"pq: connection refused\""

// logBurstApp lists the healthy orders Deployment: three replicas, all
// Running and Ready.
func logBurstApp(c *cluster) *workload {
	c.list(c.node("n1", "zone-a"))
	app := c.deployment("shop", "orders", "registry.example.com/o:5", 3)
	app.setReady(3)
	c.list(app.objects())
	c.list(app.pod(0, "n1"), app.pod(1, "n1"), app.pod(2, "n1"))
	return app
}

// sampleWindows records one sample per 2 minutes: the windows so far,
// each with its error count, the last against the ones before it.
func sampleWindows(
	c *cluster, id inventory.EntityID, counts []int, line string,
) {
	var windows [][]string
	for _, n := range counts {
		windows = append(windows, errorWindow(n, line))
		c.emit(kube.LogSampleObservation(id, c.now, windows))
		c.after(2 * time.Minute)
	}
}

func healthyPodsErrorBurst() scenario {
	return scenario{
		expect: expectation{
			Name: "healthy-pods-error-burst",
			Description: "All replicas are Running and Ready while the " +
				"log reports hundreds of errors, new against the " +
				"workload's own earlier windows.",
			Root: "deployment/shop/orders", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			app := logBurstApp(c)
			id := inventory.CoreID(kube.KindDeployment, "shop",
				app.deployment.Name)
			c.after(time.Minute)
			sampleWindows(c, id,
				[]int{1, 2, 0, 1, 2, 1, 340, 330, 310, 0, 0, 0},
				ordersRefused)
		},
	}
}

func healthyPodsSteadyErrors() scenario {
	return scenario{
		expect: expectation{
			Name: "healthy-pods-steady-errors",
			Description: "A service that always logs hundreds of " +
				"errors per window is not news when it still does.",
			Quiet: true,
		},
		build: func(c *cluster) {
			app := logBurstApp(c)
			id := inventory.CoreID(kube.KindDeployment, "shop",
				app.deployment.Name)
			c.after(time.Minute)
			sampleWindows(c, id,
				[]int{300, 280, 320, 310, 290, 300, 340, 330},
				ordersRefused)
		},
	}
}

func healthyPodsSmallBurst() scenario {
	return scenario{
		expect: expectation{
			Name: "healthy-pods-small-burst",
			Description: "Ten errors in a window after a quiet hour is " +
				"under the floor for any service.",
			Quiet: true,
		},
		build: func(c *cluster) {
			app := logBurstApp(c)
			id := inventory.CoreID(kube.KindDeployment, "shop",
				app.deployment.Name)
			c.after(time.Minute)
			sampleWindows(c, id, []int{0, 0, 0, 0, 0, 0, 10, 0, 0},
				ordersRefused)
		},
	}
}

func healthyPodsBurstWithoutBaseline() scenario {
	return scenario{
		expect: expectation{
			Name: "healthy-pods-burst-without-baseline",
			Description: "A burst in the first windows kwatch sees has " +
				"no earlier windows to be new against.",
			Quiet: true,
		},
		build: func(c *cluster) {
			app := logBurstApp(c)
			id := inventory.CoreID(kube.KindDeployment, "shop",
				app.deployment.Name)
			c.after(time.Minute)
			sampleWindows(c, id, []int{0, 0, 400, 380}, strings.Clone(
				ordersRefused))
		},
	}
}
