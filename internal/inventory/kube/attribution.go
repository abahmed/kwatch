package kube

import (
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Attribution is who made a change, as part of which release, and when
// the API server recorded it.
type Attribution struct {
	Actor    string
	App      string
	Revision string
	// At is the API server time of the write, or zero when unknown.
	At time.Time
}

// apiTimeSkew is how far an API server time may lie ahead of the receive
// time before it is treated as clock skew and ignored.
const apiTimeSkew = 2 * time.Second

// apiTimeMaxAge bounds how old a managed-fields time may be for the
// change just received. An older time belongs to an earlier write by the
// same manager, so the receive time is closer to the truth.
const apiTimeMaxAge = 5 * time.Minute

// Attribute describes the write that produced obj. created selects the
// creation timestamp as the API server time.
func Attribute(obj any, created bool) Attribution {
	meta, ok := obj.(metav1.Object)
	if !ok {
		return Attribution{}
	}
	actor := SpecManager(meta)
	if actor == "" {
		actor = Actor(meta)
	}
	out := Attribution{
		Actor: actor, App: GitOpsApp(meta), Revision: Revision(obj),
	}
	if created {
		out.At = meta.GetCreationTimestamp().Time
	} else {
		out.At = latestSpecTime(meta)
	}
	return out
}

// changeTime picks the API server time when it is plausible for a change
// received at receivedAt, and receivedAt otherwise.
func changeTime(apiTime, receivedAt time.Time) time.Time {
	if apiTime.IsZero() || apiTime.After(receivedAt.Add(apiTimeSkew)) ||
		receivedAt.Sub(apiTime) > apiTimeMaxAge {
		return receivedAt
	}
	return apiTime
}

func latestSpecTime(meta metav1.Object) time.Time {
	entries := meta.GetManagedFields()
	if index := latestSpecWrite(entries); index >= 0 {
		return entries[index].Time.Time
	}
	return time.Time{}
}

// GitOps markers, from the tools' documented labels and annotations.
const (
	argoInstanceLabel   = "argocd.argoproj.io/instance"
	argoTrackingID      = "argocd.argoproj.io/tracking-id"
	fluxKustomizeLabel  = "kustomize.toolkit.fluxcd.io/name"
	fluxHelmLabel       = "helm.toolkit.fluxcd.io/name"
	helmReleaseName     = "meta.helm.sh/release-name"
	helmChartLabel      = "helm.sh/chart"
	deploymentRevision  = "deployment.kubernetes.io/revision"
	daemonSetGeneration = "deprecated.daemonset.template.generation"
)

// gitOpsMarker maps one label or annotation to the tool that set it.
type gitOpsMarker struct {
	key  string
	tool string
}

// gitOpsMarkers are checked in order; the first present one wins. A
// GitOps controller is named before Helm because it usually renders the
// Helm chart itself.
var gitOpsMarkers = []gitOpsMarker{
	{argoInstanceLabel, "argocd"},
	{argoTrackingID, "argocd"},
	{fluxKustomizeLabel, "flux"},
	{fluxHelmLabel, "flux"},
	{helmReleaseName, "helm"},
	{helmChartLabel, "helm"},
}

// GitOpsApp names the application that deploys meta, such as
// "argocd/shop", "flux/apps" or "helm/api", or returns "".
func GitOpsApp(meta metav1.Object) string {
	labels, annotations := meta.GetLabels(), meta.GetAnnotations()
	for _, marker := range gitOpsMarkers {
		value := labels[marker.key]
		if value == "" {
			value = annotations[marker.key]
		}
		if value == "" {
			continue
		}
		if marker.key == argoTrackingID {
			// The tracking ID is "app:group/kind:namespace/name".
			value, _, _ = strings.Cut(value, ":")
		}
		return marker.tool + "/" + value
	}
	return ""
}

// Revision returns the object's revision: the Deployment or DaemonSet
// revision, the StatefulSet update revision, a ConfigMap or Secret data
// hash, or the generation. It is empty when none is known.
func Revision(obj any) string {
	switch typed := obj.(type) {
	case *appsv1.StatefulSet:
		if typed.Status.UpdateRevision != "" {
			return typed.Status.UpdateRevision
		}
	case *appsv1.ControllerRevision:
		return strconv.FormatInt(typed.Revision, 10)
	case *corev1.ConfigMap:
		return mapDigest(configMapDigests(typed))
	case *corev1.Secret:
		return mapDigest(digests(typed.Data))
	}
	meta, ok := obj.(metav1.Object)
	if !ok {
		return ""
	}
	for _, key := range []string{deploymentRevision, daemonSetGeneration} {
		if value := meta.GetAnnotations()[key]; value != "" {
			return value
		}
	}
	if generation := meta.GetGeneration(); generation > 0 {
		return "generation " + strconv.FormatInt(generation, 10)
	}
	return ""
}

// attributedChange builds the change for obj received at receivedAt.
func attributedChange(
	obj any, created bool, receivedAt time.Time,
	fields []inventory.FieldChange,
) inventory.Change {
	who := Attribute(obj, created)
	return inventory.Change{
		At: changeTime(who.At, receivedAt), Observed: receivedAt,
		Actor: who.Actor, App: who.App, Revision: who.Revision,
		Created: created, Fields: fields,
	}
}
