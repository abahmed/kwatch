package detectors

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/inventory"
)

// Entry is one production detector and the capability that entitles it.
// Governs, when set, names the capability of a single finding reason, for
// a detector whose findings belong to more than one capability; a reason
// it does not name belongs to Feature. Also lists every other capability
// Governs can return.
type Entry struct {
	Feature  feature.ID
	Detector detection.Detector
	Governs  func(reason string) feature.ID
	Also     []feature.ID
}

// Entries returns the production detector set, in the order the
// registry evaluates them, with default thresholds, each with the
// capability that entitles it.
func Entries() []Entry {
	return []Entry{
		{Feature: feature.PodDetection, Detector: Container{},
			Governs: containerGoverns,
			Also:    []feature.ID{feature.PodReadiness}},
		{Feature: feature.PodDetection, Detector: InitWait{}},
		{Feature: feature.CustomResources, Detector: Generic{}},
		{Feature: feature.PodDetection, Detector: NewPod(PodThresholds{}),
			Governs: podGoverns, Also: []feature.ID{
				feature.PodReadiness, feature.PodAdvanced}},
		{Feature: feature.NodeDetection, Detector: NewNode(0),
			Governs: nodeGoverns, Also: []feature.ID{feature.NodeUsage}},
		{Feature: feature.WorkloadDetection, Detector: NewWorkload(0)},
		{Feature: feature.JobFailures, Detector: Job{}},
		{Feature: feature.Autoscaling, Detector: HPA{}},
		{Feature: feature.StorageDetection, Detector: Claim{}},
		{Feature: feature.StorageDetection, Detector: Volume{}},
		{Feature: feature.NetworkDetection, Detector: Service{}},
		{Feature: feature.CertificateDetection, Detector: Certificate{}},
		{Feature: feature.ConfigurationRisks, Detector: Missing{}},
		{Feature: feature.UnusualEvents, Detector: Event{}},
		{Feature: feature.DisruptionBudgets, Detector: Budget{}},
		{Feature: feature.QuotaDetection, Detector: Quota{}},
		{Feature: feature.StorageDetection, Detector: Attachment{}},
		{Feature: feature.AdmissionDetection, Detector: Webhook{}},
		{Feature: feature.NodeUsage, Detector: NodeUsage{}},
		{Feature: feature.VolumeUsage, Detector: VolumeUsage{}},
		{Feature: feature.CustomResources, Detector: Custom{}},
		{Feature: feature.ControlPlane, Detector: ClusterService{}},
		{Feature: feature.NetworkDetection, Detector: Ingress{}},
		{Feature: feature.NetworkDetection, Detector: EgressPolicy{}},
		{Feature: feature.JobFailures, Detector: Schedule{}},
		{Feature: feature.WorkloadDetection, Detector: Namespace{}},
		{Feature: feature.ConfigurationRisks,
			Detector: ContainerResources{}},
		{Feature: feature.StorageDetection, Detector: PodStorage{}},
		{Feature: feature.NodeUsage, Detector: NodeHealth{}},
		{Feature: feature.ControlPlane, Detector: VersionSkew{}},
		{Feature: feature.ActiveProbes, Detector: ActiveProbe{}},
		{Feature: feature.NodeUsage, Detector: NodeCommitment{}},
		{Feature: feature.ConfigurationRisks, Detector: Risk{}},
		{Feature: feature.ConfigurationRisks, Detector: ImageDrift{}},
		{Feature: feature.ControlPlane, Detector: Lease{}},
		{Feature: feature.ControlPlane, Detector: DeprecatedAPI{}},
		{Feature: feature.ControlPlane, Detector: APIServerLoad{}},
		{Feature: feature.AdmissionDetection, Detector: WebhookCalls{}},
		{Feature: feature.ChangeCorrelation, Detector: ReleaseWatch{}},
	}
}

// podGoverns assigns the sustained readiness finding of a pod and the
// rare pod reasons to their own capabilities; every other pod finding is
// core pod detection.
func podGoverns(reason string) feature.ID {
	switch reason {
	case reasons.ContainersNotReady:
		return feature.PodReadiness
	case reasons.PodPreempted, reasons.PodPreemptedRepeatedly,
		reasons.PodAdmissionRejected, reasons.PodKilledAtGrace,
		reasons.SchedulingGated:
		return feature.PodAdvanced
	}
	return feature.PodDetection
}

// containerGoverns assigns a failing readiness probe to pod readiness,
// with the sustained readiness finding it precedes. Liveness and startup
// probe failures stay core pod detection.
func containerGoverns(reason string) feature.ID {
	if reason == reasons.ReadinessProbeFailed {
		return feature.PodReadiness
	}
	return feature.PodDetection
}

// nodeGoverns keeps NotReady with node detection. Pressure, draining,
// stuck deletion and the other node conditions come from kubelet stats
// and node state, which are the node usage capability.
func nodeGoverns(reason string) feature.ID {
	if reason == reasons.NodeNotReady {
		return feature.NodeDetection
	}
	return feature.NodeUsage
}

// Default returns the production detector set, in the order the
// registry evaluates them, with default thresholds. Replay, the scenario
// library and every test harness build their registry from it, so a
// detector added here is detected everywhere. It is the Pro set; see
// Entitled for what an edition runs.
func Default() []detection.Detector {
	entries := Entries()
	out := make([]detection.Detector, len(entries))
	for i, entry := range entries {
		out[i] = entry.Detector
	}
	return out
}

// Entitled returns the detectors an installation may run. A detector
// whose capability is not entitled is left out; one whose findings
// belong to several capabilities is wrapped so only the entitled
// findings come through, and the withheld ones are counted in withheld
// (optional). A nil e is the Community edition.
func Entitled(
	e feature.Entitlements, withheld *feature.Withheld,
) []detection.Detector {
	e = feature.OrCommunity(e)
	var out []detection.Detector
	for _, entry := range Entries() {
		det := entry.Detector
		if node, ok := det.(Node); ok &&
			!e.Has(drainFeature()) {
			det = node.WithDrainDropped()
		}
		switch {
		case !e.Has(entry.Feature):
		case entry.Governs == nil || entitlesAll(e, entry.Also):
			out = append(out, det)
		default:
			out = append(out, restricted{Detector: det,
				entitled: e, governs: entry.Governs,
				withheld: withheld})
		}
	}
	return out
}

func entitlesAll(e feature.Entitlements, ids []feature.ID) bool {
	for _, id := range ids {
		if !e.Has(id) {
			return false
		}
	}
	return true
}

// restricted passes through only the findings of entitled capabilities.
type restricted struct {
	detection.Detector
	entitled feature.Entitlements
	governs  func(reason string) feature.ID
	withheld *feature.Withheld
}

// Detect implements detection.Detector.
func (r restricted) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	found := r.Detector.Detect(ctx, e)
	kept := found[:0:0]
	for _, f := range found {
		if r.entitled.Has(r.governs(f.Reason)) {
			kept = append(kept, f)
			continue
		}
		if category, ok := withheldCategory(f.Reason); ok {
			r.withheld.Record(ctx.Now, category,
				e.ID.String()+"\x00"+f.Reason)
		}
	}
	return kept
}

// withheldCategory is the category a withheld Community finding is
// counted under; ok is false for a finding that is not counted.
func withheldCategory(reason string) (feature.WithheldCategory, bool) {
	switch reason {
	case reasons.ContainersNotReady, reasons.ReadinessProbeFailed:
		return feature.WithheldReadiness, true
	case reasons.SchedulingGated:
		return feature.WithheldGates, true
	case reasons.PodPreempted, reasons.PodPreemptedRepeatedly,
		reasons.PodAdmissionRejected, reasons.PodKilledAtGrace:
		return feature.WithheldPreemption, true
	}
	return 0, false
}
