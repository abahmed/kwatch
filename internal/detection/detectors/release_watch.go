package detectors

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Release watch thresholds. They are constants, not settings: a release
// that restarts this much is wrong on any service.
const (
	// releaseWatchWindow is how long a new revision is compared with the
	// one it replaced. Later restarts are the container detectors' job.
	releaseWatchWindow = 15 * time.Minute
	// releaseWorseFactor is how many times the previous restart rate the
	// new revision must reach. Below it the difference is ordinary noise.
	releaseWorseFactor = 3.0
	// releaseMinRestarts is the least number of restarts of the new
	// revision that is judged at all, so a quiet service whose rate
	// goes from 0 to one restart is not called a regression.
	releaseMinRestarts = 3
	// releaseQuietAfter is how long after the window the release stays
	// reported without a new restart before the finding clears.
	releaseQuietAfter = 10 * time.Minute
	// releaseRecheck is how often a watched release is looked at again:
	// restarts of its pods do not touch the Deployment.
	releaseRecheck = time.Minute
	// releaseMinPodAge keeps a pod that was created seconds ago from
	// making the rate absurdly high.
	releaseMinPodAge = time.Minute
)

// ReleaseWatch compares a Deployment's new revision with the one it
// replaced, for releaseWatchWindow after the rollout. The previous
// revision's pods are gone by then, so its restart rate comes from the
// workload's rolling baseline (restarts per hour, hours without
// restarts included), which the pipeline filled before the rollout.
type ReleaseWatch struct{}

// Name implements detection.Detector.
func (ReleaseWatch) Name() string { return "release-watch" }

// Kinds implements detection.Detector.
func (ReleaseWatch) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindDeployment}
}

// Detect implements detection.Detector.
func (ReleaseWatch) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if ctx.Model == nil || flag(e, kube.AttrDeleting) {
		return nil
	}
	release, ok := latestRelease(ctx.Model, e.ID)
	if !ok || len(release.pods) == 0 {
		return nil
	}
	age := ctx.Now.Sub(release.started())
	release.revisionAge = min(age, releaseWatchWindow)
	if age < 0 || !watched(ctx, release, age) {
		return nil
	}
	if alreadyReported(e, release) {
		return nil
	}
	count, hours := release.restartRate(ctx.Now)
	if count < releaseMinRestarts || hours <= 0 {
		return nil
	}
	replicas, _ := number(e, kube.AttrReplicas)
	before := previousRate(ctx.Model, e.ID, max(replicas, 1))
	after := float64(count) / hours
	if after < releaseWorseFactor*before {
		return nil
	}
	return []detection.Finding{{
		Reason: reasons.ReleaseRegression, Severity: detection.Warning,
		Since:    release.started(),
		Summary:  releaseSummary(release, count, before, after),
		Evidence: releaseEvidence(release, count, before, after),
	}}
}

// watched reports whether the release is still judged. Inside the window
// it always is; after it, only while its containers keep restarting, so
// a release that goes on failing is not announced as healthy when the
// window ends. It also asks for the next look, because pod restarts do
// not re-evaluate the Deployment.
func watched(ctx detection.Context, r release, age time.Duration) bool {
	if age < releaseWatchWindow {
		ctx.RecheckAfter(min(releaseRecheck, releaseWatchWindow-age))
		return true
	}
	last := r.lastRestart()
	if last.IsZero() || ctx.Now.Sub(last) >= releaseQuietAfter {
		return false
	}
	ctx.RecheckAfter(releaseRecheck)
	return true
}

// alreadyReported is true when an existing detector speaks for the new
// revision: the rollout missed its progress deadline, or a container is
// already crash-looping or restarting enough for its own finding.
func alreadyReported(e inventory.Entity, r release) bool {
	if _, reason, _ := condition(e, "Progressing"); reason ==
		reasons.ProgressDeadlineExceeded {
		return true
	}
	for _, c := range r.containers() {
		restarts, _ := number(c, kube.AttrRestarts)
		if restarts >= highRestarts ||
			text(c, kube.AttrStateReason) == reasons.CrashLoopBackOff {
			return true
		}
	}
	return false
}

// previousRate is the restarts per pod-hour the workload had before the
// rollout: the mean of its recent hourly samples over its replicas. A
// workload with no restart history had none.
func previousRate(
	model inventory.Reader, id inventory.EntityID, replicas float64,
) float64 {
	history, ok := model.(inventory.HistoryReader)
	if !ok {
		return 0
	}
	stat, ok := history.Baselines().Stat(id, inventory.MetricRestartsPerHour)
	if !ok || len(stat.Recent) == 0 {
		return 0
	}
	sum := 0.0
	for _, hour := range stat.Recent {
		sum += hour
	}
	return sum / float64(len(stat.Recent)) / replicas
}

func releaseSummary(r release, count int, before, after float64) string {
	line := "Rollout " + r.revision + " restarted " +
		countText(float64(count)) + " times in its first " + r.age() +
		" minutes"
	if before <= 0 {
		return line + "; rollout " + r.previous + " did not restart"
	}
	return fmt.Sprintf("%s, %.0f times as often as rollout %s did",
		line, after/before, r.previous)
}

func releaseEvidence(
	r release, count int, before, after float64,
) []detection.Evidence {
	out := []detection.Evidence{
		{Label: "new revision", Value: r.revision},
		{Label: "previous revision", Value: r.previous},
		{Label: "new restarts", Value: strconv.Itoa(count)},
		{Label: "new rate", Value: rateText(after)},
		{Label: "previous rate", Value: rateText(before)},
	}
	if r.image != "" && r.previousImage != "" && r.image != r.previousImage {
		out = append(out,
			detection.Evidence{Label: "image before", Value: r.previousImage},
			detection.Evidence{Label: "image after", Value: r.image})
	}
	return out
}

func rateText(perPodHour float64) string {
	return strings.TrimRight(strings.TrimRight(
		fmt.Sprintf("%.2f", perPodHour), "0"), ".") + " restarts per pod-hour"
}
