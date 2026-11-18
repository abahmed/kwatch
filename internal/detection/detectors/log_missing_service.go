package detectors

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// LogServiceMissing detects a workload whose log, sampled while its
// pods are Ready, keeps failing to resolve a name shaped like a Service
// of the cluster (kube.AttrLogUnresolvedName, recorded only once seen
// in several windows over a while) while no Service of that name exists
// in a namespace that does. It is a low finding: the pods are Ready,
// and the call fails every time. It clears when the Service appears or
// the lookups stop. A cluster DNS that fails itself answers nothing for
// any name, so then nothing is said.
type LogServiceMissing struct{}

// Name implements detection.Detector.
func (LogServiceMissing) Name() string { return "log-service-missing" }

// Kinds implements detection.Detector.
func (LogServiceMissing) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindDeployment, kube.KindStatefulSet,
		kube.KindDaemonSet}
}

// Detect implements detection.Detector.
func (LogServiceMissing) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	logged := text(e, kube.AttrLogUnresolvedName)
	if logged == "" || ctx.Model == nil || !ctx.Synced(kube.KindService) ||
		clusterDNSFailing(ctx) {
		return nil
	}
	name, namespace, _ := strings.Cut(logged, ".")
	service := inventory.CoreID(kube.KindService, namespace, name)
	if ctx.Model.Exists(service) || !ctx.Model.Exists(
		inventory.CoreID(kube.KindNamespace, "", namespace)) {
		return nil
	}
	return []detection.Finding{{
		Reason: reasons.LogServiceMissing, Severity: detection.Info,
		Since:    timestamp(e, kube.AttrLogUnresolvedSince),
		Summary:  "keeps trying to reach Service " + name + ", which does not exist",
		Evidence: missingServiceEvidence(ctx, e, service),
	}}
}

// missingServiceEvidence says what kwatch checked: the Service, how
// often the log showed it, a line, and a similar existing name.
func missingServiceEvidence(
	ctx detection.Context, e inventory.Entity, service inventory.EntityID,
) []detection.Evidence {
	samples, _ := number(e, kube.AttrLogUnresolvedSamples)
	out := []detection.Evidence{
		{Label: detection.EvidenceMissingService,
			Value: service.Name + "." + service.Namespace},
		{Label: detection.EvidenceLogSamples, Value: itoa(samples)},
		{Label: detection.EvidenceLogLine,
			Value: text(e, kube.AttrLogUnresolvedLine)},
	}
	var names []string
	for _, id := range ctx.Model.EntitiesIn(kube.KindService,
		service.Namespace) {
		names = append(names, id.Name)
	}
	if near := format.ClosestName(service.Name, names); near != "" {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceSimilarService, Value: near})
	}
	return out
}

// clusterDNSFailing reports a cluster DNS kwatch's own lookup finds
// failing: every name fails then, existing or not.
func clusterDNSFailing(ctx detection.Context) bool {
	e, ok := ctx.Model.Entity(kube.ClusterDNS)
	if !ok {
		return false
	}
	healthy, known := e.Attribute(kube.AttrHealthy)
	up, _ := healthy.Value.AsBool()
	return known && !up
}
