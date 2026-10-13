package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// These quote a finding beside what the same workload did over the
// last week. They compare and never excuse: the finding stands.

// pendingJudgement compares how long a pod has waited to start with how
// long its workload's pods usually take.
func pendingJudgement(
	ctx detection.Context, pod inventory.Entity,
) judgement {
	b := baselinesOf(ctx)
	created := timestamp(pod, kube.AttrCreated)
	if b == nil || created.IsZero() ||
		!timestamp(pod, kube.AttrStartTime).IsZero() {
		return judgement{}
	}
	waited := ctx.Now.Sub(created)
	workload := inventory.TopOwner(ctx.Model, pod.ID)
	cmp := b.Compare(workload, inventory.MetricPendingSeconds,
		waited.Seconds())
	return judge(cmp, "pending for "+format.Duration(waited)+
		" vs a usual "+usualDuration(cmp.Typical))
}

// withPendingBaseline quotes the pending time on a pod's findings.
func withPendingBaseline(
	ctx detection.Context, pod inventory.Entity, found []detection.Finding,
) []detection.Finding {
	if len(found) == 0 {
		return found
	}
	j := pendingJudgement(ctx, pod)
	for i := range found {
		if found[i].Reason != reasons.SchedulingGated {
			j.quote(&found[i])
		}
	}
	return found
}

// jobJudgement compares how long a Job has run with how long its
// workload's Jobs usually take to complete.
func jobJudgement(
	ctx detection.Context, job inventory.Entity, started time.Time,
) judgement {
	b := baselinesOf(ctx)
	if b == nil || started.IsZero() {
		return judgement{}
	}
	ran := ctx.Now.Sub(started)
	workload := inventory.TopOwner(ctx.Model, job.ID)
	cmp := b.Compare(workload, inventory.MetricJobSeconds, ran.Seconds())
	return judge(cmp, "running "+format.Duration(ran)+" vs a usual "+
		usualDuration(cmp.Typical))
}
