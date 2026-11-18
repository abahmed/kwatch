package detectors

import (
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ruleRestart reports a container whose latest exit was restarted by one
// of its own restart rules on that exit code (kube.AttrRestartRule): the
// restart is what the rule asks for, not a crash. It is a crash loop
// again when the container keeps ending that way within seconds of each
// start, as a crash on start does (crashLoopRestarts restarts, a run
// shorter than crashLoopHold). A kill by the OOM killer is never one a
// rule excuses.
func ruleRestart(e inventory.Entity) bool {
	if !flag(e, kube.AttrRestartRule) {
		return false
	}
	reason := text(e, kube.AttrLastReason)
	if text(e, kube.AttrState) == "terminated" {
		reason = text(e, kube.AttrStateReason)
	}
	if reason == reasons.OOMKilled {
		return false
	}
	restarts, _ := number(e, kube.AttrRestarts)
	run, known := number(e, kube.AttrRestartRuleRun)
	onStart := known && run < crashLoopHold.Seconds()
	return restarts < crashLoopRestarts || !onStart
}

// allContainersRestarting is the pod condition the kubelet sets while a
// RestartAllContainers rule restarts every container of the pod in
// place: the others are stopped on purpose, not crashing.
const allContainersRestarting = "AllContainersRestarting"

func restartingAllContainers(pod inventory.Entity) bool {
	status, _, _ := condition(pod, allContainersRestarting)
	return status == "True"
}
