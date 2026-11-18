package kube

import (
	"sort"
	"strconv"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// KindHelmRelease is one Helm release, as the labels of the Secrets Helm
// stores its revisions in describe it. Only labels and the creation time
// are read: the release payload (the chart and values) is never fetched.
const KindHelmRelease inventory.Kind = "helmrelease"

// Attributes of a Helm release, from its latest stored revision.
const (
	// AttrHelmStatus is the latest revision's status label: deployed,
	// failed, pending-install, pending-upgrade, pending-rollback,
	// superseded, uninstalling or uninstalled.
	AttrHelmStatus = "helm.status"
	// AttrHelmRevision is the latest revision's number.
	AttrHelmRevision = "helm.revision"
	// AttrHelmStatusSince is when the latest revision got its status:
	// Helm's modifiedAt label, else the Secret's creation time.
	AttrHelmStatusSince = "helm.status.since"
	// AttrHelmGoodRevision is the newest earlier revision that was
	// deployed (now deployed or superseded); absent when none is.
	AttrHelmGoodRevision = "helm.good.revision"
	// AttrHelmRevisionCreated is when Helm stored the latest revision:
	// when the install, upgrade or rollback that made it began. A
	// revision that failed got its status later, when Helm gave up.
	AttrHelmRevisionCreated = "helm.revision.created"
)

// Labels Helm writes on every release Secret (storage driver "secret").
const (
	helmOwnerLabel    = "owner"
	helmNameLabel     = "name"
	helmStatusLabel   = "status"
	helmVersionLabel  = "version"
	helmModifiedLabel = "modifiedAt"
)

// HelmRevision is one stored revision of a Helm release, read from the
// labels of its Secret.
type HelmRevision struct {
	Namespace string
	Release   string
	Status    string
	Version   int
	Since     time.Time
	// Created is when the revision's Secret was created.
	Created time.Time
}

// HelmRevisionOf reads the revision a Helm release Secret stores from
// its labels. ok is false for a Secret Helm did not write.
func HelmRevisionOf(meta metav1.Object) (HelmRevision, bool) {
	labels := meta.GetLabels()
	version, err := strconv.Atoi(labels[helmVersionLabel])
	if labels[helmOwnerLabel] != "helm" || labels[helmNameLabel] == "" ||
		err != nil || version <= 0 {
		return HelmRevision{}, false
	}
	since := meta.GetCreationTimestamp().Time
	if unix, err := strconv.ParseInt(labels[helmModifiedLabel], 10,
		64); err == nil && unix > 0 {
		since = time.Unix(unix, 0).UTC()
	}
	return HelmRevision{
		Namespace: meta.GetNamespace(), Release: labels[helmNameLabel],
		Status: labels[helmStatusLabel], Version: version, Since: since,
		Created: meta.GetCreationTimestamp().Time,
	}, true
}

// HelmReleaseID is the entity of a release.
func HelmReleaseID(namespace, release string) inventory.EntityID {
	return inventory.CoreID(KindHelmRelease, namespace, release)
}

// HelmReleaseObservation describes a release from its stored revisions,
// all of one release. With no revision left the release is gone.
func HelmReleaseObservation(
	id inventory.EntityID, revisions []HelmRevision, at time.Time,
) inventory.Observation {
	if len(revisions) == 0 {
		return goneObservation(id, at)
	}
	sorted := append([]HelmRevision(nil), revisions...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Version > sorted[j].Version
	})
	latest := sorted[0]
	attrs := map[string]inventory.Value{
		AttrHelmStatus:   inventory.Text(latest.Status),
		AttrHelmRevision: inventory.Number(float64(latest.Version)),
	}
	if !latest.Since.IsZero() {
		attrs[AttrHelmStatusSince] = inventory.Time(latest.Since)
	}
	if !latest.Created.IsZero() {
		attrs[AttrHelmRevisionCreated] = inventory.Time(latest.Created)
	}
	for _, r := range sorted[1:] {
		if r.Status == "deployed" || r.Status == "superseded" {
			attrs[AttrHelmGoodRevision] = inventory.Number(
				float64(r.Version))
			break
		}
	}
	return inventory.Observation{
		Kind: inventory.Observed, Source: ObservationSource, At: at,
		Entity: id, Attributes: attrs,
	}
}
