package explain

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ExposureWindow bounds how far before a run's start an edit of what it
// reads still counts. The causal window answers "what changed shortly
// before the failure"; a Job run reads its Secrets and ConfigMaps when
// it starts, which may be hours after the edit and is the first moment
// the edit could matter.
const ExposureWindow = 24 * time.Hour

// changeFrom is where the window of id's changes begins: the causal
// window of the earliest failure that reached id, or the window that
// ends now when none did. A ConfigMap or Secret reaches back further
// when something that failed read it long after the edit (see
// exposureFrom).
func (v *view) changeFrom(id inventory.EntityID) time.Time {
	from := v.since
	if reached, ok := v.reachFrom[id]; ok && reached.Before(from) {
		from = reached
	}
	if exposed, ok := v.exposureFrom(id); ok && exposed.Before(from) {
		from = exposed
	}
	return from
}

// exposureFrom is how far back an edit of a ConfigMap or Secret still
// counts because a failing user read it late. A failing Job or CronJob
// run reaches back to that run's start less ExposureWindow, but not
// before the CronJob last succeeded: a run that read the object after
// the edit and worked shows the edit harmless. A pull Secret of a pod
// whose pull was refused with an authentication error reaches back the
// same way from the pod's creation: running pods never pull again, so
// the first new pod is the first moment a bad token matters. So does
// an object named by a pod whose container the kubelet could not
// configure (see startExposure).
func (v *view) exposureFrom(id inventory.EntityID) (time.Time, bool) {
	if id.Kind != kube.KindConfigMap && id.Kind != kube.KindSecret {
		return time.Time{}, false
	}
	if cached, ok := v.exposures[id]; ok {
		return cached.t, cached.ok
	}
	var from timeValue
	for _, user := range v.s.Model.Related(
		id, inventory.References, inventory.Incoming,
	) {
		start, succeeded, ok := v.runExposure(user)
		if !ok {
			start, ok = v.pullExposure(user, id)
		}
		if !ok {
			start, ok = v.startExposure(user, id)
		}
		if !ok {
			continue
		}
		begin := start.Add(-ExposureWindow)
		if succeeded.After(begin) {
			begin = succeeded
		}
		from = from.earlier(begin)
	}
	v.exposures[id] = from
	return from.t, from.ok
}

// runExposure reads a failing Job or CronJob: when its latest run
// started and when its CronJob last succeeded (zero when it never did).
func (v *view) runExposure(
	user inventory.EntityID,
) (started, succeeded time.Time, ok bool) {
	entity, found := v.s.Model.Entity(user)
	if !found || !v.failing(user) {
		return started, succeeded, false
	}
	cron := entity
	switch user.Kind {
	case kube.KindJob:
		started = attributeTime(entity, kube.AttrCreated)
		for _, owner := range v.s.Model.Related(
			user, inventory.OwnedBy, inventory.Outgoing,
		) {
			if owner.Kind == kube.KindCronJob {
				cron, _ = v.s.Model.Entity(owner)
			}
		}
	case kube.KindCronJob:
		started = attributeTime(entity, kube.AttrLastSchedule)
	default:
		return started, succeeded, false
	}
	succeeded = attributeTime(cron, kube.AttrLastSuccess)
	return started, succeeded, !started.IsZero()
}

// pullExposure reads a pod whose image pull was refused for want of a
// valid login and that names the Secret as an image pull Secret: when
// it was created. Healthy peers say nothing here, since they pulled
// before the edit; the window is only longer, never narrowed.
func (v *view) pullExposure(
	user, secret inventory.EntityID,
) (created time.Time, ok bool) {
	if user.Kind != kube.KindPod || !v.unitFailing(user) {
		return created, false
	}
	e, found := v.s.Model.Entity(user)
	if !found || !hasText(kube.PullSecretNames(attrText(e,
		kube.AttrPullSecrets)), secret.Name) {
		return created, false
	}
	if !v.pullRefused(user) {
		return created, false
	}
	created = attributeTime(e, kube.AttrCreated)
	return created, !created.IsZero()
}

// pullRefused reports a pod, or a container of it, whose failed pulls
// show an authentication answer.
func (v *view) pullRefused(pod inventory.EntityID) bool {
	for _, id := range append([]inventory.EntityID{pod},
		v.s.Model.Related(pod, inventory.PartOf, inventory.Incoming)...) {
		if text, ok := v.pullText(id); ok &&
			classifyRegistryPull(text) == pullAuth {
			return true
		}
	}
	return false
}

// startExposure reads a pod whose container the kubelet could not
// configure and whose error names the object: when it was created. A
// key read into the environment is resolved when a container starts and
// at no other time, so an edit breaks nothing until the next pod
// starts, which may be hours later; that pod's creation is the first
// moment the edit could matter. The kubelet naming the object is what
// ties the old edit to this failure.
func (v *view) startExposure(
	user, object inventory.EntityID,
) (created time.Time, ok bool) {
	if user.Kind != kube.KindPod || !v.unitFailing(user) ||
		!v.kubeletNames(user, object) {
		return created, false
	}
	e, found := v.s.Model.Entity(user)
	if !found {
		return created, false
	}
	created = attributeTime(e, kube.AttrCreated)
	return created, !created.IsZero()
}
