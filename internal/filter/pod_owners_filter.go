package filter

import (
	apiv1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/observe"
)

type PodOwnersFilter struct{}

func (f PodOwnersFilter) Detect(ctx *Context) Status {
	return StatusAlert
}

// Enrich resolves the workload a pod belongs to and records it on the
// context, so every detector downstream keys its findings the same way.
//
// The walk up the owner chain used to be written out here a second time,
// beside the one the monitors use. Two implementations of "who owns this
// pod" is how a kubelet-derived incident and a crash incident about the same
// Deployment ended up under different keys, and neither could resolve the
// other. There is one walk now, in observe.PodOwners; this only decides which
// caches it may read.
func (f PodOwnersFilter) Enrich(ctx *Context) bool {
	if ctx.Owner != nil || ctx.Pod == nil {
		return false
	}
	if len(ctx.Pod.OwnerReferences) == 0 {
		return false
	}
	owner := observe.PodOwners{
		RS: ctx.RSLister,
		DS: ctx.DSLister,
		SS: ctx.SSLister,
		// The API fallback matters here and only here: the filters run in
		// unit tests and in a kwatch whose informers are not wired, where a
		// lister is nil but the client works.
		Client: ctx.Client,
		Ctx:    ctx.Ctx,
	}.OwnerOf(ctx.Pod)
	if owner.Name == "" {
		// Unresolved. Leaving ctx.Owner nil is the honest answer: the
		// detectors then key by the pod rather than by a guess.
		return false
	}
	ctx.Owner = &apiv1.OwnerReference{Kind: owner.Kind, Name: owner.Name}
	return false
}

func (f PodOwnersFilter) Execute(ctx *Context) bool {
	return f.Enrich(ctx)
}
