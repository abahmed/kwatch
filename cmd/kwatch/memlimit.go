package main

import (
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

// memoryLimitEnv carries the container memory limit in bytes, set by the
// manifests through the downward API (resourceFieldRef limits.memory).
const memoryLimitEnv = "KWATCH_MEMORY_LIMIT"

// memoryLimitPercent is the share of the container limit the Go runtime
// aims to stay under. The rest is headroom for memory the Go heap limit
// does not cover (stacks, cgroup page cache) so the kernel OOM killer is
// not the first to act.
const memoryLimitPercent = 90

// applyMemoryLimit sets the Go soft memory limit to 90% of the container
// limit. An explicit GOMEMLIMIT always wins, and a missing or invalid
// limit leaves the runtime default. It returns the limit it set, or 0.
func applyMemoryLimit(getenv func(string) string) int64 {
	if getenv("GOMEMLIMIT") != "" {
		return 0
	}
	limit, err := strconv.ParseInt(
		strings.TrimSpace(getenv(memoryLimitEnv)), 10, 64)
	if err != nil || limit <= 0 {
		return 0
	}
	soft := limit / 100 * memoryLimitPercent
	debug.SetMemoryLimit(soft)
	return soft
}

// setMemoryLimitFromEnv applies the limit from the process environment.
func setMemoryLimitFromEnv() { applyMemoryLimit(os.Getenv) }
