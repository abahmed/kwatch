package detectors

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// currentAPIVersion is the version that replaces the old ones of an API
// group. It is a short, fixed list from the Kubernetes deprecation
// guide (kubernetes.io/docs/reference/using-api/deprecation-guide):
// nearly every group ended at v1, autoscaling at v2. A group missing
// here, or one whose resources moved to different groups
// (extensions/v1beta1), gets "the newer version" instead of a guess.
var currentAPIVersion = map[string]string{
	"policy":                       "v1",
	"batch":                        "v1",
	"autoscaling":                  "v2",
	"networking.k8s.io":            "v1",
	"storage.k8s.io":               "v1",
	"rbac.authorization.k8s.io":    "v1",
	"admissionregistration.k8s.io": "v1",
	"apiextensions.k8s.io":         "v1",
	"apiregistration.k8s.io":       "v1",
	"certificates.k8s.io":          "v1",
	"coordination.k8s.io":          "v1",
	"discovery.k8s.io":             "v1",
	"events.k8s.io":                "v1",
	"node.k8s.io":                  "v1",
	"scheduling.k8s.io":            "v1",
	"flowcontrol.apiserver.k8s.io": "v1",
}

// DeprecatedAPI reports an API version that something in the cluster
// still requests although Kubernetes will remove it. The API server
// counts requests per version, not per caller, so the finding says that
// something calls it and where to look, never who.
//
// The API server sets the metric when the version is requested and keeps
// it until it restarts, so the finding clears after a restart or a
// release upgrade, not the moment the caller is fixed.
type DeprecatedAPI struct{}

// Name implements detection.Detector.
func (DeprecatedAPI) Name() string { return "deprecated-api" }

// Kinds implements detection.Detector.
func (DeprecatedAPI) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindDeprecatedAPI}
}

// Detect implements detection.Detector.
func (DeprecatedAPI) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	removed := text(e, kube.AttrDeprecatedRemoved)
	removedMinor, ok := kube.ParseMinor(removed)
	if !ok {
		return nil
	}
	since := valueSince(e, kube.AttrDeprecatedRemoved)
	if !sustained(ctx, "deprecated-api", since, DefaultProbeFailing) {
		return nil
	}
	group := text(e, kube.AttrDeprecatedGroup)
	version := text(e, kube.AttrDeprecatedVersion)
	api := apiVersion(group, version) + " " +
		text(e, kube.AttrDeprecatedResource)
	instead := "the newer version"
	if current, ok := currentAPIVersion[group]; ok {
		instead = apiVersion(group, current)
	}
	when, what := removalText(ctx, removed, removedMinor)
	return []detection.Finding{{
		Reason: reasons.DeprecatedAPIInUse, Severity: detection.Info,
		Since:   since,
		Summary: when,
		Evidence: []detection.Evidence{
			{Label: "what it means", Value: "Something still calls " +
				api + ", which " + what + " in Kubernetes " + removed},
			{Label: "use instead", Value: instead},
			{Label: "find the caller", Value: "Search the API server " +
				"audit log for " + api + " requests, then update the " +
				"manifests and clients that send them"},
		},
	}}
}

// removalText describes the removal against the cluster's own version:
// "removed in 1.25" when it is far off, "removed in 1.25, the next
// minor upgrade" when it is one or two away, and a statement that the
// calls already fail when the cluster has reached it.
func removalText(
	ctx detection.Context, removed string, removedMinor int,
) (when, what string) {
	api, ok := ctx.Model.Entity(kube.APIServer)
	current, known := number(api, kube.AttrServerMinor)
	if !ok || !known {
		return "removed in " + removed, "is removed"
	}
	switch gap := removedMinor - int(current); {
	case gap <= 0:
		return "already removed in " + removed + "; calls fail",
			"was removed"
	case gap == 1:
		return "removed in " + removed + ", the next minor upgrade",
			"is removed"
	case gap == 2:
		return "removed in " + removed + ", two minor upgrades away",
			"is removed"
	}
	return "removed in " + removed, "is removed"
}

// apiVersion is "policy/v1beta1", or "v1" in the core group.
func apiVersion(group, version string) string {
	if group == "" {
		return version
	}
	return group + "/" + version
}
