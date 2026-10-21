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

// changeFrom is where the window of id's changes begins. A ConfigMap or
// Secret that a failing Job or CronJob run reads reaches back to that
// run's start less ExposureWindow, but not before the CronJob last
// succeeded: a run that read the object after the edit and worked shows
// the edit harmless. A pull Secret of a pod whose pull was refused with
// an authentication error reaches back the same way from the pod's
// creation: running pods never pull again, so the first new pod is the
// first moment a bad token matters.
func (v *view) changeFrom(id inventory.EntityID) time.Time {
	from := v.since
	if id.Kind != kube.KindConfigMap && id.Kind != kube.KindSecret {
		return from
	}
	for _, user := range v.s.Model.Related(
		id, inventory.References, inventory.Incoming,
	) {
		start, succeeded, ok := v.runExposure(user)
		if !ok {
			start, ok = v.pullExposure(user, id)
		}
		if !ok {
			continue
		}
		begin := start.Add(-ExposureWindow)
		if succeeded.After(begin) {
			begin = succeeded
		}
		if begin.Before(from) {
			from = begin
		}
	}
	return from
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
