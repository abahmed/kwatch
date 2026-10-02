package scenarios

import (
	"fmt"
	"strings"
	"time"
)

// sharedErrorScenarios are many workloads failing with one error that
// names no Kubernetes object: an endpoint outside the cluster, or one
// message every crash shares. They must become one incident, not one
// per workload, while a single failing workload still blames itself.
func sharedErrorScenarios() []scenario {
	return []scenario{
		externalDatabaseStorm(), externalDatabaseOneWorkload(),
		sharedPanicStorm(), stackFrameNotEndpoint(),
	}
}

// sharedDatabase is the database outside the cluster the storms call.
const sharedDatabase = "db.example.com:5432"

// databaseRefused is how a caller of sharedDatabase crashes.
const databaseRefused = "dial tcp " + sharedDatabase +
	": connect: connection refused"

// externalDatabaseStorm: the managed database every service uses stops
// accepting connections. Six Deployments of two replicas each, in two
// namespaces, crash-loop with "dial tcp db.example.com:5432: connect:
// connection refused". The endpoint is the root, and the storm is one
// incident.
func externalDatabaseStorm() scenario {
	apps := []string{"orders/api", "orders/worker", "orders/notifier",
		"billing/ledger", "billing/invoicer", "billing/exporter"}
	notBlamed := []string{"node//s1", "node//s2"}
	for _, app := range apps {
		notBlamed = append(notBlamed, "deployment/"+app)
	}
	return scenario{
		expect: expectation{
			Name: "external-database-storm",
			Description: "Six Deployments in two namespaces crash-loop " +
				"because the database outside the cluster refuses " +
				"connections.",
			Root: "external-endpoint//" + sharedDatabase, Tier: "notify",
			MaxMessages: 3, MustNotBlame: notBlamed,
		},
		build: func(c *cluster) {
			c.list(c.node("s1", "zone-a"), c.node("s2", "zone-b"))
			var fleet []*workload
			for _, app := range apps {
				fleet = append(fleet, sharedErrorApp(c, app))
			}
			c.after(3 * time.Minute)
			sharedErrorCrash(c, fleet, func(int) string {
				return databaseRefused
			})
		},
	}
}

// externalDatabaseOneWorkload: the same refused connection, but only
// the reporting job's two replicas fail on it; nothing else calls the
// database. One workload failing on an endpoint is that workload's
// problem (its own configuration, its own credentials), so it blames
// itself rather than the endpoint.
func externalDatabaseOneWorkload() scenario {
	return scenario{
		expect: expectation{
			Name: "external-database-one-workload",
			Description: "One Deployment's two replicas crash-loop on a " +
				"refused database connection; no other workload fails.",
			Root: "deployment/reports/builder", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//s1", "node//s2"},
		},
		build: func(c *cluster) {
			c.list(c.node("s1", "zone-a"), c.node("s2", "zone-b"))
			builder := sharedErrorApp(c, "reports/builder")
			sharedErrorApp(c, "reports/viewer")
			c.after(3 * time.Minute)
			sharedErrorCrash(c, []*workload{builder}, func(int) string {
				return databaseRefused
			})
		},
	}
}

// sharedPanicStorm: a license server change makes four services panic
// at start-up, each naming its own tenant: "panic: license check failed
// for tenant 17", "... tenant 23" and so on. The number differs, the
// error is the same: one incident rooted at the shared error.
func sharedPanicStorm() scenario {
	apps := []string{"tenants/router", "tenants/billing",
		"tenants/search", "tenants/reports"}
	notBlamed := []string{"node//s1", "node//s2"}
	for _, app := range apps {
		notBlamed = append(notBlamed, "deployment/"+app)
	}
	return scenario{
		expect: expectation{
			Name: "shared-panic-storm",
			Description: "Four Deployments panic with \"license check " +
				"failed for tenant <N>\", each with its own tenant " +
				"number.",
			Root: "failure-signature//CrashLoop panic: license check " +
				"failed for tenant #",
			Tier: "notify", MaxMessages: 3, MustNotBlame: notBlamed,
		},
		build: func(c *cluster) {
			c.list(c.node("s1", "zone-a"), c.node("s2", "zone-b"))
			var fleet []*workload
			for _, app := range apps {
				fleet = append(fleet, sharedErrorApp(c, app))
			}
			c.after(3 * time.Minute)
			tenants := []int{17, 23, 4, 108}
			sharedErrorCrash(c, fleet, func(i int) string {
				return fmt.Sprintf("panic: license check failed for "+
					"tenant %d", tenants[i])
			})
		},
	}
}

// hikariFrame is a JDBC pool crash whose stack frame reads like a
// host and port: "HikariPool.java:512".
const hikariFrame = "java.net.ConnectException: Connection refused\n" +
	"\tat com.zaxxer.hikari.pool.HikariPool.java:512"

// stackFrameNotEndpoint: two Deployments crash-loop with a refused
// JDBC connection whose only "host:port" is a stack frame. A source
// file and line is not an endpoint, so each app is its own root and no
// external endpoint is blamed.
func stackFrameNotEndpoint() scenario {
	return scenario{
		expect: expectation{
			Name: "stack-frame-not-endpoint",
			Description: "Two Deployments crash with a refused " +
				"connection whose stack frame HikariPool.java:512 " +
				"looks like host:port.",
			Root:       "deployment/orders/api",
			OtherRoots: []string{"deployment/billing/ledger"},
			Tier:       "notify", MaxMessages: 4,
			MustNotBlame: []string{"node//s1", "node//s2",
				"external-endpoint//com.zaxxer.hikari.pool." +
					"hikaripool.java:512"},
		},
		build: func(c *cluster) {
			c.list(c.node("s1", "zone-a"), c.node("s2", "zone-b"))
			api := sharedErrorApp(c, "orders/api")
			ledger := sharedErrorApp(c, "billing/ledger")
			c.after(3 * time.Minute)
			sharedErrorCrash(c, []*workload{api, ledger},
				func(int) string { return hikariFrame })
		},
	}
}

// sharedErrorApp lists a healthy two-replica Deployment named
// "namespace/name", one replica on each node.
func sharedErrorApp(c *cluster, app string) *workload {
	namespace, name, _ := strings.Cut(app, "/")
	w := c.deployment(namespace, name,
		"registry.example.com/"+name+":2.4", 2)
	c.list(w.objects())
	c.list(w.pod(0, "s1"), w.pod(1, "s2"))
	return w
}

// sharedErrorCrash crash-loops both replicas of every workload, once a
// minute for five minutes; message gives workload i's error.
func sharedErrorCrash(c *cluster, fleet []*workload,
	message func(i int) string) {
	for restarts := int32(1); restarts <= 5; restarts++ {
		for i, w := range fleet {
			for replica, node := range []string{"s1", "s2"} {
				c.update(w.pod(replica, node,
					crashLoop(1, "Error", message(i), restarts)))
			}
			w.setReady(0)
			c.update(w.objects())
		}
		c.after(time.Minute)
	}
}
