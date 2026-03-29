package filter

import (
	"context"

	apiv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodOwnersFilter struct{}

func (f PodOwnersFilter) Execute(ctx *Context) bool {
	if ctx.Owner != nil {
		return false
	}

	if len(ctx.Pod.OwnerReferences) == 0 {
		return false
	}

	owner := ctx.Pod.OwnerReferences[0]
	if owner.Kind == "ReplicaSet" {
		rs, _ :=
			ctx.Client.AppsV1().ReplicaSets(ctx.Pod.Namespace).Get(
				context.TODO(),
				owner.Name,
				apiv1.GetOptions{})

		if rs != nil && len(rs.ObjectMeta.OwnerReferences) > 0 {
			owner = rs.ObjectMeta.OwnerReferences[0]
		}
	} else if owner.Kind == "DaemonSet" {
		ds, _ :=
			ctx.Client.AppsV1().DaemonSets(ctx.Pod.Namespace).Get(
				context.TODO(),
				owner.Name,
				apiv1.GetOptions{})
		if ds != nil && len(ds.ObjectMeta.OwnerReferences) > 0 {
			owner = ds.ObjectMeta.OwnerReferences[0]
		}
	} else if owner.Kind == "StatefulSet" {
		ss, _ :=
			ctx.Client.AppsV1().StatefulSets(ctx.Pod.Namespace).Get(
				context.TODO(),
				owner.Name,
				apiv1.GetOptions{})
		if ss != nil && len(ss.ObjectMeta.OwnerReferences) > 0 {
			owner = ss.ObjectMeta.OwnerReferences[0]
		}
	}

	ctx.Owner = &owner

	return false
}
