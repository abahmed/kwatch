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
}

// scenarioCosts lists every covered test. A test missing here is treated as
// a 90 second scenario that runs alone, which is slow but safe.
// TestScenarioCostsMatchHelpers fails when Alone disagrees with the helper
// the test really uses.
var scenarioCosts = map[string]scenarioCost{
	"TestScenarioActiveProbeFailureAndRecovery":  {260, true},
	"TestScenarioConfigurationReload":            {3, true},
	"TestScenarioCronJobSuspended":               {82, false},
	"TestScenarioDaemonSetFailure":               {87, false},
	"TestScenarioDeploymentRolloutFailure":       {88, false},
	"TestScenarioExtendedAdmissionWebhook":       {147, true},
	"TestScenarioExtendedClusterDNSDown":         {232, true},
	"TestScenarioExtendedMetricsAPIFailure":      {207, true},
	"TestScenarioExtendedTLS":                    {82, false},
	"TestScenarioExtendedVolumeAttachment":       {214, true},
	"TestScenarioHeartbeatDelivery":              {2, true},
	"TestScenarioInvalidLiveConfiguration":       {61, true},
	"TestScenarioInvalidStartupConfiguration":    {87, true},
	"TestScenarioJobFailure":                     {84, false},
	"TestScenarioLeaseHandover":                  {91, true},
	"TestScenarioMissingConfigMapReference":      {87, false},
	"TestScenarioMissingIngressBackend":          {142, false},
	"TestScenarioMissingRequiredReferences":      {87, false},
	"TestScenarioMissingServiceAccountReference": {87, false},
	"TestScenarioNodePressureSignals":            {84, true},
	"TestScenarioNodeRecovery":                   {184, true},
	// Kwatch raises a PDB violation only after ten minutes. It runs
	// beside other scenarios, but no shard can finish before it does.
	"TestScenarioPDBDisruption":                {687, false},
	"TestScenarioPersistentVolumeClaimFailure": {131, false},
	"TestScenarioPodCrashLoop":                 {87, false},
	"TestScenarioPodEphemeralStorageEviction":  {94, false},
	"TestScenarioPodFailureProfiles":           {177, false},
	"TestScenarioPodLifecycleHookFailure":      {87, false},
	"TestScenarioPodLivenessFailure":           {91, false},
	"TestScenarioPodOOMKilled":                 {90, false},
	"TestScenarioPodReadinessFailure":          {117, false},
	"TestScenarioPodSecurityAdmission":         {6, false},
	"TestScenarioPodStartupFailureProfiles":    {89, false},
	"TestScenarioPodUnschedulable":             {202, false},
	"TestScenarioProviderFailureRecovery":      {210, true},
	"TestScenarioRefailureAfterRecovery":       {345, false},
	"TestScenarioReplicaSetFailure":            {82, false},
	"TestScenarioResolution":                   {264, false},
	"TestScenarioRestartPersistence":           {91, true},
	"TestScenarioRestrictiveNetworkPolicy":     {82, false},
	"TestScenarioRootCauseSharedNode":          {180, true},
	"TestScenarioRootCauseSharedRegistry":      {88, false},
	"TestScenarioRootCauseSmallStorm":          {89, true},
	"TestScenarioServiceWithoutEndpoints":      {142, false},
	"TestScenarioStatefulSetFailure":           {89, false},
}

// parallelSlots is how many side-by-side scenarios one shard runs; it
// matches the default SCENARIO_PARALLEL of scripts/test-kind-scenarios.sh.
const parallelSlots = 6

const unknownScenarioSeconds = 90

func costOf(test string) scenarioCost {
	if cost, ok := scenarioCosts[test]; ok {
		return cost
	}
	return scenarioCost{unknownScenarioSeconds, true}
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
	for _, test := range tests {
		best := 0
		for shard := range loads {
			cost := costOf(test)
			if loads[shard].with(cost).finish() <
				loads[best].with(cost).finish() {
				best = shard
			}
		}
		plan[test] = best
		loads[best] = loads[best].with(costOf(test))
	}
	return plan
}
