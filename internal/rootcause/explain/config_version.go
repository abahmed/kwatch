package explain

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// ConfigVersionWeight rewards a changed ConfigMap or Secret that splits
// the pods of one workload: those on one version fail, those on the
// other are healthy. The pods differ in exactly the content that
// changed, which is stronger than timing alone and weaker than an error
// that names the object.
const ConfigVersionWeight = 0.15

func init() {
	// Like the counterfactual checks, it compares with healthy pods and
	// runs after the other evidence.
	scorers = append(scorers, scorer{"config-version", scoreConfigVersion})
}

// versionSplit counts the pods of one workload by the content they read
// of a changed object. Pods that restarted since the change may have
// read either, so they are left out.
type versionSplit struct {
	oldFailing, oldHealthy int
	newFailing, newHealthy int
}

// scoreConfigVersion asks, for a ConfigMap or Secret that changed,
// whether the pods that read the new content fail while the older ones
// run healthy, or the reverse. Only pods that read the object when a
// container starts (environment, subPath) can run old content; plain
// volume mounts update in place and are never compared.
func scoreConfigVersion(v *view, c *candidate) outcome {
	if c.id.Kind != kube.KindConfigMap && c.id.Kind != kube.KindSecret {
		return outcome{}
	}
	changed := v.latestChange(c.id)
	if changed.IsZero() {
		return outcome{}
	}
	for _, u := range v.failingUnits(c) {
		owner, ok := v.directOwner(u.pods[0])
		if !ok {
			continue
		}
		split := v.splitByVersion(owner, c.id, changed)
		if o, found := versionOutcome(c.id, split); found {
			return o
		}
	}
	return outcome{}
}

// splitByVersion sorts the pods an owner has, that read config when
// they start, by the content they read and by health.
func (v *view) splitByVersion(
	owner, config inventory.EntityID, changed time.Time,
) versionSplit {
	var s versionSplit
	for _, pod := range sampleIDs(v.ownedPods(owner)) {
		e, ok := v.s.Model.Entity(pod)
		if !ok || !kube.PodFreezes(e, config) {
			continue
		}
		failing := v.unitFailing(pod)
		healthy := !failing && v.healthyPod(pod)
		switch kube.PodConfigAge(e, changed) {
		case kube.ConfigBefore:
			s.oldFailing += btoi(failing)
			s.oldHealthy += btoi(healthy)
		case kube.ConfigAfter:
			s.newFailing += btoi(failing)
			s.newHealthy += btoi(healthy)
		}
	}
	return s
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// versionOutcome turns a split into evidence: one side fails entirely
// and the other side has healthy pods and no failing one.
func versionOutcome(config inventory.EntityID, s versionSplit) (outcome, bool) {
	name := fmt.Sprintf("%s %s", config.Kind, config.Name)
	switch {
	case s.newFailing > 0 && s.newHealthy == 0 &&
		s.oldFailing == 0 && s.oldHealthy > 0:
		return outcome{weight: ConfigVersionWeight,
			code:  rootcause.ProofNewConfigFails,
			count: s.newFailing, total: s.oldHealthy,
			text: fmt.Sprintf("the %d pods started after %s changed "+
				"fail; the %d older ones are healthy", s.newFailing,
				name, s.oldHealthy)}, true
	case s.oldFailing > 0 && s.oldHealthy == 0 &&
		s.newFailing == 0 && s.newHealthy > 0:
		return outcome{weight: ConfigVersionWeight,
			code:  rootcause.ProofOldConfigFails,
			count: s.oldFailing, total: s.newHealthy,
			text: fmt.Sprintf("the %d pods still on the old %s "+
				"fail; the %d started since are healthy", s.oldFailing,
				name, s.newHealthy)}, true
	}
	return outcome{}, false
}
