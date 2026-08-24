package filter

import (
	apiv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

type PodOwnersFilter struct{}

func (f PodOwnersFilter) Detect(ctx *Context) Status {
	return StatusAlert
}

// adoptGrandparent replaces owner with the first owner reference of the
// workload fetched via get, reporting whether resolution still holds.
func adoptGrandparent(owner *apiv1.OwnerReference, namespace, label string, get func() (apiv1.Object, error)) bool {
	obj, err := get()
	if err != nil {
		klog.ErrorS(err, label, "name", owner.Name, "namespace", namespace)
		return false
	}
	if refs := obj.GetOwnerReferences(); len(refs) > 0 {
		*owner = refs[0]
	}
	return true
}

func (f PodOwnersFilter) Enrich(ctx *Context) bool {
	if ctx.Owner != nil || ctx.Pod == nil {
		return false
	}
	if len(ctx.Pod.OwnerReferences) == 0 {
		return false
	}

	owner := ctx.Pod.OwnerReferences[0]
	resolved := true

	switch owner.Kind {
	case "ReplicaSet":
		if ctx.RSLister != nil {
			resolved = adoptGrandparent(&owner, ctx.Pod.Namespace, "failed to get ReplicaSet via lister", func() (apiv1.Object, error) {
				return ctx.RSLister.ReplicaSets(ctx.Pod.Namespace).Get(owner.Name)
			})
		} else {
			resolved = adoptGrandparent(&owner, ctx.Pod.Namespace, "failed to get ReplicaSet via API", func() (apiv1.Object, error) {
				return ctx.Client.AppsV1().ReplicaSets(ctx.Pod.Namespace).Get(
					ctx.Ctx,
					owner.Name,
					apiv1.GetOptions{})
			})
		}
	case "DaemonSet":
		if ctx.DSLister != nil {
			resolved = adoptGrandparent(&owner, ctx.Pod.Namespace, "failed to get DaemonSet via lister", func() (apiv1.Object, error) {
				return ctx.DSLister.DaemonSets(ctx.Pod.Namespace).Get(owner.Name)
			})
		} else {
			resolved = adoptGrandparent(&owner, ctx.Pod.Namespace, "failed to get DaemonSet via API", func() (apiv1.Object, error) {
				return ctx.Client.AppsV1().DaemonSets(ctx.Pod.Namespace).Get(
					ctx.Ctx,
					owner.Name,
					apiv1.GetOptions{})
			})
		}
	case "StatefulSet":
		if ctx.SSLister != nil {
			resolved = adoptGrandparent(&owner, ctx.Pod.Namespace, "failed to get StatefulSet via lister", func() (apiv1.Object, error) {
				return ctx.SSLister.StatefulSets(ctx.Pod.Namespace).Get(owner.Name)
			})
		} else {
			resolved = adoptGrandparent(&owner, ctx.Pod.Namespace, "failed to get StatefulSet via API", func() (apiv1.Object, error) {
				return ctx.Client.AppsV1().StatefulSets(ctx.Pod.Namespace).Get(
					ctx.Ctx,
					owner.Name,
					apiv1.GetOptions{})
			})
		}
	}

	if resolved {
		ctx.Owner = &owner
	}
	return false
}

func (f PodOwnersFilter) Execute(ctx *Context) bool {
	return f.Enrich(ctx)
}
