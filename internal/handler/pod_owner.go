package handler

import (
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// contextOwner is the workload the filter pipeline already resolved for this
// pod, as a reference rather than a name-and-kind pair.
//
// A zero reference means "not resolved": the pod has owner references but the
// workload behind them could not be read. That must stay distinct from "no
// owner", which is a real answer -- an ownerless pod is its own logical
// owner, which keeps independent ownerless pods apart while still letting
// correlation fold a generated replacement into its predecessor's incident.
func contextOwner(ctx *filter.Context) model.ObjectRef {
	if ctx == nil || ctx.Pod == nil {
		return model.ObjectRef{}
	}
	if ctx.Owner != nil {
		return model.ObjectRef{
			Kind:      ctx.Owner.Kind,
			Namespace: ctx.Pod.Namespace,
			Name:      ctx.Owner.Name,
		}
	}
	if len(ctx.Pod.OwnerReferences) == 0 {
		return observe.SelfOwner("Pod", ctx.Pod.Namespace, ctx.Pod.Name)
	}
	return model.ObjectRef{}
}
