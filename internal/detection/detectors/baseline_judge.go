package detectors

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A judgement sets what a finding shows against what its workload
// usually does, from the workload's rolling baseline. It looks only at
// the past week. Without enough history there is no judgement, and the
// finding stands as the detector wrote it.

// judgement is the verdict and the phrase that shows the numbers.
type judgement struct {
	normal detection.Normality
	phrase string
}

// apply puts the judgement on f.
func (j judgement) apply(f *detection.Finding) {
	if j.phrase == "" {
		return
	}
	f.Normal = j.normal
	f.Evidence = append(f.Evidence, detection.Evidence{
		Label: detection.EvidenceBaseline, Value: j.phrase})
}

// judge turns a comparison into a judgement. A figure that is neither
// inside the normal range nor far above it is quoted and left alone.
func judge(cmp inventory.Comparison, phrase string) judgement {
	switch {
	case !cmp.Known:
		return judgement{}
	case cmp.Unusual:
		return judgement{detection.NormalUnusual, phrase}
	case cmp.Usual:
		return judgement{detection.NormalUsual, phrase}
	}
	return judgement{detection.NormalUnjudged, phrase}
}

// keepUsual drops a Usual verdict, for a finding that the baseline may
// never excuse: it still shows the numbers.
func (j judgement) keepUsual(allowed bool) judgement {
	if !allowed && j.normal == detection.NormalUsual {
		j.normal = detection.NormalUnjudged
	}
	return j
}

// baselinesOf returns the model's baselines, or nil for a model that
// keeps none.
func baselinesOf(ctx detection.Context) *inventory.Baselines {
	history, ok := ctx.Model.(inventory.HistoryReader)
	if !ok {
		return nil
	}
	return history.Baselines()
}

// workloadOfContainer finds the top-level controller of the container's
// pod.
func workloadOfContainer(
	ctx detection.Context, e inventory.Entity,
) (inventory.EntityID, bool) {
	pod, ok := owningPod(ctx, e)
	if !ok {
		return inventory.EntityID{}, false
	}
	return inventory.TopOwner(ctx.Model, pod.ID), true
}

// restartJudgement compares the workload's restarts in the last hour
// with its usual restarts per hour.
func restartJudgement(
	ctx detection.Context, e inventory.Entity,
) judgement {
	b := baselinesOf(ctx)
	workload, ok := workloadOfContainer(ctx, e)
	if b == nil || !ok {
		return judgement{}
	}
	rate := b.RestartsInLastHour(workload, ctx.Now)
	if rate <= 0 {
		return judgement{}
	}
	cmp := b.Compare(workload, inventory.MetricRestartsPerHour, rate)
	return judge(cmp, "restarts "+perHour(rate)+" vs a usual "+
		perHour(cmp.Typical))
}

// memoryJudgement compares bytes, a container's memory, with the usual
// hourly peak of its workload.
func memoryJudgement(
	ctx detection.Context, e inventory.Entity, bytes float64,
) judgement {
	b := baselinesOf(ctx)
	workload, ok := workloadOfContainer(ctx, e)
	if b == nil || !ok || bytes <= 0 {
		return judgement{}
	}
	cmp := b.Compare(workload, inventory.MetricMemoryPeak, bytes)
	return judge(cmp, "memory "+quantity(bytes)+" vs a usual "+
		quantity(cmp.Typical)+" peak")
}

// readyJudgement compares how long a pod that never became ready has
// been waiting with how long its workload's pods usually take. A pod
// that was ready and then failed is not starting, so its age says
// nothing about startup.
func readyJudgement(
	ctx detection.Context, pod inventory.Entity, unreadySince time.Time,
) judgement {
	b := baselinesOf(ctx)
	created := timestamp(pod, kube.AttrCreated)
	if b == nil || created.IsZero() || unreadySince.IsZero() ||
		unreadySince.Sub(created) > startSlack {
		return judgement{}
	}
	waited := ctx.Now.Sub(created)
	workload := inventory.TopOwner(ctx.Model, pod.ID)
	cmp := b.Compare(workload, inventory.MetricReadySeconds,
		waited.Seconds())
	return judge(cmp, "readiness taking "+format.Duration(waited)+
		" vs a usual "+usualDuration(cmp.Typical))
}

// startSlack is how long after a pod's creation its not-ready condition
// may have been set and still be the one it started with.
const startSlack = 2 * time.Minute

func usualDuration(seconds float64) string {
	return format.Duration(time.Duration(seconds * float64(time.Second)))
}

// perHour writes a rate such as "12×/h" or "0.1×/h".
func perHour(rate float64) string {
	switch {
	case rate == 0:
		return "0×/h"
	case rate >= 10:
		return fmt.Sprintf("%.0f×/h", rate)
	case rate >= 0.1:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", rate), ".0") + "×/h"
	}
	return fmt.Sprintf("%.2f×/h", rate)
}

// withRestartBaseline shows each restart-driven finding of the container
// beside the workload's usual restart rate. Only the plain "keeps
// restarting" finding may be demoted to the digest because the rate is
// usual: a crash loop, a kill by the OOM killer or a failing start is
// never excused by history.
func withRestartBaseline(
	ctx detection.Context, e inventory.Entity, found []detection.Finding,
) []detection.Finding {
	if len(found) == 0 {
		return found
	}
	j := restartJudgement(ctx, e)
	for i := range found {
		excuse := found[i].Reason == reasons.HighRestartCount
		j.keepUsual(excuse).apply(&found[i])
		if found[i].Reason == reasons.OOMKilled {
			quoteMemory(ctx, e, &found[i])
		}
	}
	return found
}

// quoteMemory adds the container's 24-hour memory peak beside the usual
// peak of its workload to an OOM kill. It only quotes: a kill is never
// excused.
func quoteMemory(
	ctx detection.Context, e inventory.Entity, f *detection.Finding,
) {
	peak, ok := number(e, kube.AttrMemoryPeak24h)
	if !ok {
		return
	}
	memoryJudgement(ctx, e, peak).quote(f)
}

// quote adds the phrase without changing the finding's normality.
func (j judgement) quote(f *detection.Finding) {
	if j.phrase != "" {
		f.Evidence = append(f.Evidence, detection.Evidence{
			Label: detection.EvidenceBaseline, Value: j.phrase})
	}
}
