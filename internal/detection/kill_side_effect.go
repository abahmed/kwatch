package detection

// killSideEffects are the kubelet's events about stopping a container:
// its preStop hook failed, the kill itself failed, or the container
// outlived its grace period. They follow a kill (a liveness probe that
// gave up, an eviction, a rollout) and say how the stop went, never
// why the workload fails, so they never lead an incident that has a
// probe, exit or readiness failure.
var killSideEffects = map[string]bool{
	"FailedPreStopHook":   true,
	"FailedKillPod":       true,
	"ExceededGracePeriod": true,
}

// IsKillSideEffect reports whether reason is a killSideEffects event.
func IsKillSideEffect(reason string) bool { return killSideEffects[reason] }
