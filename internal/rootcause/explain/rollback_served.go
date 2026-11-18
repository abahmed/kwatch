package explain

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// previousServed reports positive evidence that the version a rollback
// of the blamed workload (blamedWorkload) returns to has served: for a
// Deployment its ReplicaSet has ready replicas or one of its pods is
// ready, for a StatefulSet or DaemonSet a pod made from its
// ControllerRevision is ready (revisionServed), or kwatch remembers
// seeing that revision serve while it was current (rememberedServing),
// as after a completed rollout scaled it to zero. Existing and not
// failing is not enough: a revision kwatch never saw ready leaves no
// record of it running, so kwatch does not suggest going back to it.
func (s Snapshot) previousServed(workload inventory.EntityID) bool {
	if deployment, ok := deploymentOf(s.Model, workload); ok {
		rs, ok := s.previousSet(deployment)
		return ok && (setServed(s.Model, rs) ||
			s.rememberedServing(deployment, rs.Name))
	}
	w, ok := revisionWorkloadOf(s.Model, workload)
	if !ok {
		return false
	}
	cr, revision := controllerPrevious(s.Model, w)
	return revision != "" && (revisionServed(s.Model, w, cr) ||
		s.rememberedServing(w, cr.Name))
}

// previousSet is the ReplicaSet a rollback of the Deployment returns
// to: the one with the highest revision below the newest, or, when the
// sets state no revision, the one kwatch saw appear last before the
// newest.
func (s Snapshot) previousSet(
	deployment inventory.EntityID,
) (inventory.EntityID, bool) {
	if rs, revision := deploymentPrevious(s.Model, deployment); revision != "" {
		return rs, true
	}
	newest, ok := s.newestReplicaSet(deployment)
	if !ok {
		return inventory.EntityID{}, false
	}
	var earlier []inventory.EntityID
	for _, rs := range s.Model.Related(deployment, inventory.OwnedBy,
		inventory.Incoming) {
		if rs.Kind == kube.KindReplicaSet && rs != newest {
			earlier = append(earlier, rs)
		}
	}
	return s.lastSeenSet(earlier)
}

// setServed reports a ReplicaSet with ready replicas, or with a pod
// that is ready.
func setServed(model inventory.Reader, rs inventory.EntityID) bool {
	if a, ok := model.Attribute(rs, kube.AttrReadyReplicas); ok {
		if ready, _ := a.Value.AsNumber(); ready > 0 {
			return true
		}
	}
	for _, pod := range model.Related(rs, inventory.OwnedBy,
		inventory.Incoming) {
		if pod.Kind != kube.KindPod {
			continue
		}
		if a, ok := model.Attribute(pod, kube.AttrReady); ok {
			if ready, _ := a.Value.AsBool(); ready {
				return true
			}
		}
	}
	return false
}

// helmGoodRevision is the revision a Helm rollback of the release that
// deploys the changed workload returns to: the newest earlier revision
// Helm recorded as deployed (kube.AttrHelmGoodRevision), never simply
// the one before, which may have failed too. Empty when Helm does not
// deploy it (change.App is not "helm/<release>" or
// "helm/<namespace>/<release>") or kwatch does not see the release.
func helmGoodRevision(model inventory.Reader, change inventory.Change) string {
	tool, release, _ := strings.Cut(change.App, "/")
	if tool != "helm" || release == "" {
		return ""
	}
	space := change.Entity.Namespace
	if s, r, ok := strings.Cut(release, "/"); ok {
		space, release = s, r
	}
	a, ok := model.Attribute(kube.HelmReleaseID(space, release),
		kube.AttrHelmGoodRevision)
	if !ok {
		return ""
	}
	if good, _ := a.Value.AsNumber(); good > 0 {
		return strconv.Itoa(int(good))
	}
	return ""
}
