package filter

import (
	"context"

	apiv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

type PodOwnersFilter struct{}

func (f PodOwnersFilter) Detect(ctx *Context) Status {
	return StatusAlert
}

func (f PodOwnersFilter) Enrich(ctx *Context) bool {
	if ctx.Owner != nil {
		return false
	}
	if len(ctx.Pod.OwnerReferences) == 0 {
		return false
	}

	owner := ctx.Pod.OwnerReferences[0]
	resolved := true

	if owner.Kind == "ReplicaSet" {
		if ctx.RSLister != nil {
			rs, err := ctx.RSLister.ReplicaSets(ctx.Pod.Namespace).Get(owner.Name)
			if err != nil {
				klog.ErrorS(err, "failed to get ReplicaSet via lister", "name", owner.Name, "namespace", ctx.Pod.Namespace)
				resolved = false
			} else if len(rs.ObjectMeta.OwnerReferences) > 0 {
				owner = rs.ObjectMeta.OwnerReferences[0]
			}
		} else {
			rs, err :=
				ctx.Client.AppsV1().ReplicaSets(ctx.Pod.Namespace).Get(
					context.TODO(),
					owner.Name,
					apiv1.GetOptions{})
			if err != nil {
				klog.ErrorS(err, "failed to get ReplicaSet via API", "name", owner.Name, "namespace", ctx.Pod.Namespace)
				resolved = false
			} else if len(rs.ObjectMeta.OwnerReferences) > 0 {
				owner = rs.ObjectMeta.OwnerReferences[0]
			}
		}
	} else if owner.Kind == "DaemonSet" {
		if ctx.DSLister != nil {
			ds, err := ctx.DSLister.DaemonSets(ctx.Pod.Namespace).Get(owner.Name)
			if err != nil {
				klog.ErrorS(err, "failed to get DaemonSet via lister", "name", owner.Name, "namespace", ctx.Pod.Namespace)
				resolved = false
			} else if len(ds.ObjectMeta.OwnerReferences) > 0 {
				owner = ds.ObjectMeta.OwnerReferences[0]
			}
		} else {
			ds, err :=
				ctx.Client.AppsV1().DaemonSets(ctx.Pod.Namespace).Get(
					context.TODO(),
					owner.Name,
					apiv1.GetOptions{})
			if err != nil {
				klog.ErrorS(err, "failed to get DaemonSet via API", "name", owner.Name, "namespace", ctx.Pod.Namespace)
				resolved = false
			} else if len(ds.ObjectMeta.OwnerReferences) > 0 {
				owner = ds.ObjectMeta.OwnerReferences[0]
			}
		}
	} else if owner.Kind == "StatefulSet" {
		if ctx.SSLister != nil {
			ss, err := ctx.SSLister.StatefulSets(ctx.Pod.Namespace).Get(owner.Name)
			if err != nil {
				klog.ErrorS(err, "failed to get StatefulSet via lister", "name", owner.Name, "namespace", ctx.Pod.Namespace)
				resolved = false
			} else if len(ss.ObjectMeta.OwnerReferences) > 0 {
				owner = ss.ObjectMeta.OwnerReferences[0]
			}
		} else {
			ss, err :=
				ctx.Client.AppsV1().StatefulSets(ctx.Pod.Namespace).Get(
					context.TODO(),
					owner.Name,
					apiv1.GetOptions{})
			if err != nil {
				klog.ErrorS(err, "failed to get StatefulSet via API", "name", owner.Name, "namespace", ctx.Pod.Namespace)
				resolved = false
			} else if len(ss.ObjectMeta.OwnerReferences) > 0 {
				owner = ss.ObjectMeta.OwnerReferences[0]
			}
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
