package investigate

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// signalExit reports an exit code of 128+n: the process was ended by
// signal n (137 SIGKILL, 143 SIGTERM, 139 SIGSEGV, ...). A signal says
// how the run ended, not why: the kernel's OOM killer, a failed
// liveness probe, an eviction or a person all produce 137 or 143.
func signalExit(code string) bool {
	n, err := strconv.Atoi(code)
	return err == nil && n > 128 && n < 256
}

// signalExitUnexplained reports that nothing else accounts for a
// signal exit of its container: its termination reason is known and is
// not an out-of-memory kill, and no probe failure, probe event or
// probe-failure finding points to a kill by the kubelet. Only then may
// the exit code alone stand as a crash.
func signalExitUnexplained(
	model inventory.Reader, p incident.Incident, pod inventory.EntityID,
	e inventory.Entity,
) bool {
	reason := terminationReason(e)
	if reason == "" || strings.EqualFold(reason, "OOMKilled") {
		return false
	}
	if hasMode(p, append([]string{"OOMKilled", "Evicted"}, probeModes...)) {
		return false
	}
	if n, _ := strconv.Atoi(textOf(e, kube.AttrLivenessFailures)); n > 0 ||
		textOf(e, kube.AttrProbeFailureKind) != "" ||
		textOf(e, kube.AttrProbeError) != "" {
		return false
	}
	for _, line := range notesOf(model, p, []inventory.EntityID{pod}) {
		if strings.Contains(line, "Unhealthy") ||
			strings.Contains(line, "Killing") {
			return false
		}
	}
	return true
}

// terminationReason is the reason of the run that ended last: its last
// termination once the container restarted, else its current state.
func terminationReason(e inventory.Entity) string {
	if r := textOf(e, kube.AttrLastReason); r != "" {
		return r
	}
	if textOf(e, kube.AttrLastExitCode) == "" {
		return textOf(e, kube.AttrStateReason)
	}
	return ""
}
