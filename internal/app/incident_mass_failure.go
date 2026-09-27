package app

import (
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

// massFailureHook reports new mass failures and resolves ones that cleared.
func massFailureHook(opts *engineOptions, holder *engineHolder) func() {
	var mu sync.Mutex
	seen := make(map[string]time.Time)
	return func() {
		mu.Lock()
		defer mu.Unlock()
		allIncidents := holder.engine.ActiveIncidents()
		incList := make([]*model.Incident, 0, len(allIncidents))
		for _, inc := range allIncidents {
			incList = append(incList, inc)
		}
		mfs := insight.ScanMassFailures(incList, opts.graph)

		current := make(map[string]insight.MassFailure, len(mfs))
		present := make(map[string]bool, len(mfs))
		now := holder.engine.Now()
		for _, mf := range mfs {
			if coveredByNodeIncident(mf.SharedDependency, incList) {
				continue
			}
			var supported bool
			mf, supported = supportedMassFailure(
				mf, incList, opts.insightEngine,
			)
			if !supported {
				continue
			}
			present[mf.SharedDependency] = true
			if !massFailureSustained(
				seen, mf.SharedDependency, now,
				holder.engine.HasMassFailure(
					incident.MassFailureKey(mf.SharedDependency),
				),
			) {
				continue
			}
			current[mf.SharedDependency] = mf
		}
		for dependency := range seen {
			if !present[dependency] {
				delete(seen, dependency)
			}
		}
		notifyNewMassFailures(holder, current)
		resolveClearedMassFailures(holder, current)
	}
}

const massFailureSustain = 2 * time.Minute

func massFailureSustained(
	seen map[string]time.Time,
	dependency string,
	now time.Time,
	alreadyActive bool,
) bool {
	if alreadyActive {
		return true
	}
	first, ok := seen[dependency]
	if !ok {
		seen[dependency] = now
		return false
	}
	return !now.Before(first.Add(massFailureSustain))
}

// A common graph edge is correlation. An active dependency incident or a
// recent change to that dependency provides evidence for one shared alert.
func supportedMassFailure(
	mf insight.MassFailure,
	incidents []*model.Incident,
	engine *insight.Engine,
) (insight.MassFailure, bool) {
	ref, ok := model.ParseObjectKey(mf.SharedDependency)
	if !ok {
		return mf, false
	}
	for _, inc := range incidents {
		if inc.State == model.StateActive && inc.Ref() == ref {
			return mf, true
		}
	}
	if engine == nil {
		return mf, false
	}
	mf = engine.EnrichMassFailure(mf)
	// Graph topology alone cannot establish the cause of several symptoms.
	mf.RootCause = ""
	return mf, len(mf.RecentChanges) > 0
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
		ref, ok := model.ParseObjectKey(key)
		if !ok {
			continue
		}
		klog.V(2).InfoS("mass failure detected", "message", description)
		inc := &model.Incident{
			Subject: model.Subject{
				ID:        incident.IncidentID(incKey),
				Key:       incKey,
				Reason:    constant.ReasonSharedDependencyFailure,
				Namespace: ref.Namespace,
				Resource:  ref.Kind,
				Name:      ref.Name,
				Object:    ref,
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
