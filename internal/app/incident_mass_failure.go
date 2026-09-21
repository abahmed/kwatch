package app

import (
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

// massFailureHook reports new mass failures and resolves ones that cleared.
func massFailureHook(opts *engineOptions, holder *engineHolder) func() {
	return func() {
		allIncidents := holder.engine.ActiveIncidents()
		incList := make([]*model.Incident, 0, len(allIncidents))
		for _, inc := range allIncidents {
			incList = append(incList, inc)
		}
		mfs := insight.ScanMassFailures(incList, opts.graph)

		current := make(map[string]insight.MassFailure, len(mfs))
		for _, mf := range mfs {
			if coveredByNodeIncident(mf.SharedDependency, incList) {
				continue
			}
			current[mf.SharedDependency] = mf
		}
		notifyNewMassFailures(holder, current)
		resolveClearedMassFailures(holder, current)
	}
}

// coveredByNodeIncident avoids announcing the same node failure twice.
func coveredByNodeIncident(depKey string, incidents []*model.Incident) bool {
	const nodePrefix = "node//"
	if !strings.HasPrefix(depKey, nodePrefix) {
		return false
	}
	node := strings.TrimPrefix(depKey, nodePrefix)
	for _, inc := range incidents {
		if inc.Ref() == (model.ObjectRef{Kind: "node", Name: node}) {
			return true
		}
	}
	return false
}

// notifyNewMassFailures fires incidents for failures not yet tracked.
func notifyNewMassFailures(
	holder *engineHolder,
	current map[string]insight.MassFailure,
) {
	for key, mf := range current {
		incKey := incident.MassFailureKey(key)
		if holder.engine.HasMassFailure(incKey) {
			continue
		}
		now := holder.engine.Now()
		description := mf.DescribeAt(now)
		klog.V(2).InfoS("mass failure detected", "message", description)
		inc := &model.Incident{
			Subject: model.Subject{
				ID:        incident.IncidentID(incKey),
				Key:       incKey,
				Reason:    mf.Reason,
				Namespace: mf.Namespace,
				Resource:  mf.ResourceKind,
				Name:      describeDependency(key),
			},
			Status: model.Status{
				Count:         mf.AffectedCount,
				PeakResources: mf.AffectedCount,
				FirstSeen:     now,
				LastSeen:      now,
				State:         model.StateActive,
			},
			Evidence: model.Evidence{Hint: description},
		}

		holder.engine.AddMassFailure(inc)
	}
}

// describeDependency turns an internal dependency key into readable text.
func describeDependency(depKey string) string {
	ref, ok := model.ParseObjectKey(depKey)
	if !ok {
		return depKey
	}
	return ref.Describe()
}

// resolveClearedMassFailures resolves tracked failures no longer present.
func resolveClearedMassFailures(
	holder *engineHolder,
	current map[string]insight.MassFailure,
) {
	tracked := holder.engine.MassFailureSet()
	for incKey := range tracked {
		dep := strings.TrimPrefix(string(incKey), "mass-failure/")
		if _, exists := current[dep]; exists {
			continue
		}
		klog.V(2).InfoS("mass failure resolved", "dependency", dep)
		holder.engine.RemoveMassFailure(incKey)
	}
}
