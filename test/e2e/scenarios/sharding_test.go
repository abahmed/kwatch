//go:build e2e

package scenarios

import (
	"sort"
	"time"
)

// scenarioCost is what sharding needs to know about one scenario test.
type scenarioCost struct {
	// Seconds is the measured duration (CI run 2026-10-06). Most of it is
	// Kwatch's own settle and sustain time, so it barely depends on the
	// machine. Refresh it from the "Scenario timings" CI output.
	Seconds int
	// Alone is true when the scenario runs through inNamespaceAlone or
	// onCluster. Alone scenarios run one after another before the rest.
	Alone bool
	// Disturbs names what an Alone scenario changes for everyone else, so
	// that a reader can see why it is exclusive. It is empty for scenarios
	// that stay inside their own namespace. See the lock table below.
	Disturbs string
	// Own gives the scenario a Kind cluster to itself, so nothing waits
	// for it and it waits for nothing.
	Own bool
}

// Lock table. Every Alone scenario names in Disturbs what it changes for
// the others, and Alone scenarios run one after another inside a shard:
//
//	kwatch-pod         restarts or replaces the Kwatch Pod
//	kwatch-config      changes the KwatchConfig Kwatch is running with
//	receiver           makes the webhook receiver fail or empties it
//	cluster-dns        stops CoreDNS, which every Pod depends on
//	metrics-api        breaks the metrics APIService HPAs read
//	admission-webhook  adds a webhook that sees every create
//	volume-attachment  adds a failed cluster-wide VolumeAttachment
//	worker-node        stops a worker node and its Pods
//	scheduler-load     crashes 50 Pods and floods the event stream
//
// Two scenarios that disturb different things could in principle overlap,
// but nothing proves they cannot influence each other's assertions (all of
// them read the same Kwatch output), so they stay exclusive. The speed-up
// comes from the cluster count instead: extra Kind clusters are free, so
// the balancer deals Alone scenarios over many shards and gives the
// slowest ones (Own) a whole cluster. Treat an unlisted scenario as
// exclusive.

// shared is a scenario that stays inside its own namespace.
func shared(seconds int) scenarioCost { return scenarioCost{Seconds: seconds} }

// alone is an exclusive scenario that disturbs the named resource.
func alone(seconds int, disturbs string) scenarioCost {
	return scenarioCost{Seconds: seconds, Alone: true, Disturbs: disturbs}
}

// own is an exclusive scenario that also gets a cluster to itself.
func own(seconds int, disturbs string) scenarioCost {
	c := alone(seconds, disturbs)
	c.Own = true
	return c
}

// ownShared is a namespaced scenario that gets a cluster to itself.
func ownShared(seconds int) scenarioCost {
	return scenarioCost{Seconds: seconds, Own: true}
}

// scenarioCosts lists every covered test. A test missing here is treated as
// a 90 second scenario that runs alone, which is slow but safe.
// TestScenarioCostsMatchHelpers fails when Alone disagrees with the helper
// the test really uses.
var scenarioCosts = map[string]scenarioCost{
	"TestScenarioActiveProbeFailureAndRecovery":  alone(268, "kwatch-config"),
	"TestScenarioConfigurationReload":            alone(3, "kwatch-config"),
	"TestScenarioCronJobSuspended":               shared(82),
	"TestScenarioDaemonSetFailure":               shared(87),
	"TestScenarioDeploymentRolloutFailure":       shared(88),
	"TestScenarioExtendedAdmissionWebhook":       alone(147, "admission-webhook"),
	"TestScenarioExtendedClusterDNSDown":         own(206, "cluster-dns"),
	"TestScenarioExtendedMetricsAPIFailure":      alone(203, "metrics-api"),
	"TestScenarioExtendedTLS":                    shared(82),
	"TestScenarioExtendedVolumeAttachment":       alone(198, "volume-attachment"),
	"TestScenarioHeartbeatDelivery":              alone(2, "kwatch-pod"),
	"TestScenarioInvalidLiveConfiguration":       alone(65, "kwatch-config"),
	"TestScenarioInvalidStartupConfiguration":    alone(3, "kwatch-config"),
	"TestScenarioJobFailure":                     shared(84),
	"TestScenarioLeaseHandover":                  alone(91, "kwatch-pod"),
	"TestScenarioMissingConfigMapReference":      shared(87),
	"TestScenarioMissingIngressBackend":          shared(142),
	"TestScenarioMissingRequiredReferences":      shared(87),
	"TestScenarioMissingServiceAccountReference": shared(87),
	"TestScenarioNodePressureSignals":            alone(84, "worker-node"),
	"TestScenarioNodeRecovery":                   alone(222, "worker-node"),
	// Kwatch raises a PDB violation ten minutes after it first sees the
	// budget, so this scenario sets the length of the whole run. It has a
	// Kind cluster to itself and starts as soon as Kwatch is running.
	"TestScenarioPDBDisruption":                ownShared(665),
	"TestScenarioPersistentVolumeClaimFailure": shared(131),
	"TestScenarioPodCrashLoop":                 shared(87),
	"TestScenarioPodEphemeralStorageEviction":  shared(94),
	"TestScenarioPodFailureProfiles":           shared(241),
	"TestScenarioPodLifecycleHookFailure":      shared(87),
	"TestScenarioPodLivenessFailure":           shared(91),
	"TestScenarioPodOOMKilled":                 shared(90),
	"TestScenarioPodReadinessFailure":          shared(117),
	"TestScenarioPodSecurityAdmission":         shared(6),
	"TestScenarioPodStartupFailureProfiles":    shared(89),
	"TestScenarioPodUnschedulable":             shared(202),
	"TestScenarioProviderFailureRecovery":      alone(210, "receiver"),
	"TestScenarioRefailureAfterRecovery":       shared(345),
	"TestScenarioReplicaSetFailure":            shared(82),
	"TestScenarioResolution":                   shared(264),
	"TestScenarioRestartPersistence":           alone(91, "kwatch-pod"),
	"TestScenarioRestrictiveNetworkPolicy":     shared(82),
	"TestScenarioRootCauseSharedNode":          alone(180, "worker-node"),
	"TestScenarioRootCauseSharedRegistry":      shared(88),
	"TestScenarioRootCauseSmallStorm":          alone(89, "scheduler-load"),
	"TestScenarioServiceWithoutEndpoints":      shared(142),
	"TestScenarioStatefulSetFailure":           shared(89),
}

// parallelSlots is how many side-by-side scenarios one shard runs; it
// matches the default SCENARIO_PARALLEL of scripts/test-kind-scenarios.sh.
const parallelSlots = 10

const unknownScenarioSeconds = 90

// nodeDisturbance is the Disturbs value of the scenarios that stop a worker
// node. A node incident stays open for a while after the node is back, so a
// second scenario that stops the same worker in the same cluster would only
// update that incident. Each such scenario therefore gets its own shard
// whenever there are enough shards.
const nodeDisturbance = "worker-node"

// ownShardsFrom is the smallest shard count that gives Own scenarios a
// shard each; with fewer shards they would starve the others.
const ownShardsFrom = 8

func costOf(test string) scenarioCost {
	if cost, ok := scenarioCosts[test]; ok {
		return cost
	}
	return alone(unknownScenarioSeconds, "unknown")
}

// shardLoad is the work already dealt to one shard.
type shardLoad struct {
	alone    int // seconds of scenarios that run one after another
	parallel int // seconds of scenarios that run side by side
	longest  int // the longest side-by-side scenario
}

// with returns the load after adding one more scenario.
func (l shardLoad) with(cost scenarioCost) shardLoad {
	if cost.Alone {
		l.alone += cost.Seconds
		return l
	}
	l.parallel += cost.Seconds
	l.longest = max(l.longest, cost.Seconds)
	return l
}

// finish estimates when the shard is done: the alone scenarios in a row,
// then the rest in parallelSlots lanes, but never before the longest one.
func (l shardLoad) finish() time.Duration {
	lanes := (l.parallel + parallelSlots - 1) / parallelSlots
	return time.Duration(l.alone+max(l.longest, lanes)) * time.Second
}

// shardOf picks the shard that runs scenario id. Scenarios are grouped by
// their test function (several coverage entries can share one test) and
// dealt out longest first, each to the shard that would finish earliest
// with it. An ID missing from the catalog falls back to a hash.
func shardOf(id string, total int) int {
	test, ok := scenarioTests()[id]
	if !ok {
		hash := sha1Sum(id)
		return int(hash[0]) % total
	}
	return shardPlan(total)[test]
}

// shardPlan maps every covered test to its shard.
func shardPlan(total int) map[string]int {
	var tests []string
	seen := map[string]bool{}
	for _, entry := range coveredEntries() {
		if !seen[entry.Test] {
			seen[entry.Test] = true
			tests = append(tests, entry.Test)
		}
	}
	sort.Slice(tests, func(i, j int) bool {
		a, b := costOf(tests[i]), costOf(tests[j])
		if a.Seconds != b.Seconds {
			return a.Seconds > b.Seconds
		}
		return tests[i] < tests[j]
	})
	loads := make([]shardLoad, total)
	plan := make(map[string]int, len(tests))
	// Own scenarios take the first shards, one each, when there are enough
	// shards. The rest are dealt over the other shards.
	first := 0
	var rest []string
	for _, test := range tests {
		if costOf(test).Own && total >= ownShardsFrom && first < total-1 {
			plan[test] = first
			loads[first] = loads[first].with(costOf(test))
			first++
			continue
		}
		rest = append(rest, test)
	}
	nodeShards := map[int]bool{}
	for _, test := range rest {
		cost := costOf(test)
		best := -1
		for shard := first; shard < total; shard++ {
			if cost.Disturbs == nodeDisturbance && nodeShards[shard] &&
				len(nodeShards) < total-first {
				continue
			}
			if best < 0 || loads[shard].with(cost).finish() <
				loads[best].with(cost).finish() {
				best = shard
			}
		}
		if cost.Disturbs == nodeDisturbance {
			nodeShards[best] = true
		}
		plan[test] = best
		loads[best] = loads[best].with(costOf(test))
	}
	return plan
}
