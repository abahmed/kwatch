package filter

import (
	"fmt"
	"math"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/constant"
)

// DefaultNotReadyThreshold is the floor a pod must stay not ready before an
// alert fires. It is intentionally not configurable — the detection should
// just work out of the box. For a pod that has never become ready the floor
// is raised to whatever startup budget the pod's own probes declare, so a
// slow-booting application does not alert on every rollout.
const DefaultNotReadyThreshold = 60 * time.Second

// maxStartupBudget caps how long a probe definition can defer an alert. A
// pod declaring an hour of startup budget should still be reported well
// before that.
const maxStartupBudget = 15 * time.Minute

// probeBudget is how long a probe allows a container to take before it is
// considered failed: the initial delay plus every permitted retry.
func probeBudget(p *corev1.Probe) time.Duration {
	if p == nil {
		return 0
	}
	period := p.PeriodSeconds
	if period <= 0 {
		period = 10 // Kubernetes default
	}
	failures := p.FailureThreshold
	if failures <= 0 {
		failures = 3 // Kubernetes default
	}
	return time.Duration(p.InitialDelaySeconds)*time.Second +
		time.Duration(failures)*time.Duration(period)*time.Second
}

// startupBudget returns the longest startup allowance any container in the
// pod declares. Kubernetes already knows how long the workload expects to
// take; deriving the threshold from it beats guessing a single constant for
// every workload in every cluster.
func startupBudget(pod *corev1.Pod) time.Duration {
	var longest time.Duration
	all := make(
		[]corev1.Container,
		0,
		len(pod.Spec.Containers)+len(pod.Spec.InitContainers),
	)
	all = append(all, pod.Spec.InitContainers...)
	all = append(all, pod.Spec.Containers...)
	for i := range all {
		c := &all[i]
		b := probeBudget(c.StartupProbe)
		if b == 0 {
			b = probeBudget(c.ReadinessProbe)
		}
		if b > longest {
			longest = b
		}
	}
	if longest > maxStartupBudget {
		return maxStartupBudget
	}
	return longest
}

// hasEverBeenReady reports whether the pod reached readiness at some point.
// A pod that has never been ready is still starting — a rollout — whereas one
// that was ready and stopped being ready has actually degraded. They deserve
// different patience and different wording.
//
// Kubernetes keeps no readiness history, so this is inferred: a container
// currently Ready settles it; otherwise, PodReady flipping to False well after
// the pod was created means it must have been True in between. A pod that has
// never been ready carries a PodReady=False transition stamped at creation.
// Restart count is deliberately not used — a container killed by its liveness
// probe before ever passing readiness restarts too, and that is the classic
// slow-start case this distinction exists to protect.
func hasEverBeenReady(
	pod *corev1.Pod,
	notReadySince time.Time,
	startupBudget time.Duration,
) bool {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			return true
		}
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	created := pod.CreationTimestamp.Time
	if created.IsZero() || notReadySince.IsZero() {
		return false
	}
	return notReadySince.Sub(created) > startupBudget
}

// NotReadyFilter alerts when a pod has been not ready (PodReady=False) for
// longer than Threshold even though all its containers are running and have
// not crashed. The container detectors intentionally skip running containers
// with no restarts, so the classic "readiness probe failing while the app is
// up" case would otherwise produce no alert at all.
type NotReadyFilter struct {
	Threshold time.Duration
}

func (f NotReadyFilter) Detect(ctx *Context) Status {
	if ctx.Pod == nil {
		return StatusContinue
	}
	if ctx.Pod.Status.Phase != corev1.PodRunning {
		return StatusContinue
	}

	// A container-level failure (crash, waiting, terminated with non-zero
	// exit) is handled by the container pipeline — don't double-alert here.
	for _, cs := range ctx.Pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			return StatusContinue
		}
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 &&
			cs.State.Terminated.Reason != "Completed" {
			return StatusContinue
		}
	}

	// Already alerting for this pod at pod level with the same reason.
	if ctx.PodLastState != nil &&
		ctx.PodLastState.Reason == constant.ReasonContainersNotReady {
		return StatusContinue
	}

	var lastTransition time.Time
	for _, c := range ctx.Pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			if c.Status == corev1.ConditionTrue {
				return StatusContinue
			}
			lastTransition = c.LastTransitionTime.Time
			break
		}
	}
	if lastTransition.IsZero() {
		return StatusContinue
	}

	// How long the pod has actually been unready. Reported to the user, and
	// never clamped — a pod unready for three hours must not claim one minute.
	notReadyFor := ctx.now().Sub(lastTransition)

	// A pod that has never been ready is mid-startup, so give it whatever
	// budget its own probes declare. One that was ready and degraded gets the
	// plain floor, because that is a real regression.
	budget := startupBudget(ctx.Pod)
	if budget < f.Threshold {
		budget = f.Threshold
	}
	everReady := hasEverBeenReady(ctx.Pod, lastTransition, budget)
	threshold := f.Threshold
	if !everReady {
		threshold = budget
	}

	// Separately, do not alert within one threshold of kwatch starting up:
	// on restart every pre-existing condition would otherwise fire at once.
	// This gates the alert only; it never alters the duration reported above.
	sinceWatch := time.Duration(math.MaxInt64)
	if !ctx.Config.WatchStartTime.IsZero() {
		sinceWatch = ctx.now().Sub(ctx.Config.WatchStartTime)
	}
	if notReadyFor < threshold || sinceWatch < f.Threshold {
		return StatusContinue
	}

	ctx.PodHasIssues = true
	ctx.ContainersHasIssues = false
	ctx.PodReason = constant.ReasonContainersNotReady
	if everReady {
		ctx.PodMsg = fmt.Sprintf("pod stopped being ready %s ago",
			notReadyFor.Round(time.Second))
	} else {
		ctx.PodMsg = fmt.Sprintf(
			"pod has never become ready — %s since start (allowed %s)",
			notReadyFor.Round(time.Second),
			threshold.Round(time.Second),
		)
	}

	return StatusAlert
}

func (f NotReadyFilter) Execute(ctx *Context) bool {
	return f.Detect(ctx) == StatusSkip
}
