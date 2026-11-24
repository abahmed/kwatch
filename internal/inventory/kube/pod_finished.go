package kube

import "github.com/abahmed/kwatch/internal/inventory"

// FinishedPhase reports a pod phase whose containers have all stopped
// for good: Succeeded (a completed Job or CronJob run) or Failed (an
// evicted pod, a run that failed). Such a pod is not Ready, and that
// says nothing about its node or its workload now.
func FinishedPhase(phase string) bool {
	return phase == "Succeeded" || phase == "Failed"
}

// PodFinished reports a pod in a finished phase; see FinishedPhase.
func PodFinished(pod inventory.Entity) bool {
	return FinishedPhase(entityText(pod, AttrPhase))
}
