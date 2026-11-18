package kube

import (
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Attributes that tie objects to the installation that made them, so
// the objects an operator left behind can be told from another
// operator's in the same namespace.
const (
	// AttrRelease lists, comma-separated, the installation names an
	// object carries: its app.kubernetes.io/instance and
	// app.kubernetes.io/part-of labels and its Helm release.
	AttrRelease = "release"
	// AttrCertificateName is the cert-manager Certificate that writes
	// a Secret, as "namespace/name".
	AttrCertificateName = "certificate.name"
	// AttrCAFrom is where cert-manager injects a webhook's CA bundle
	// from: "certificate:namespace/name" or "secret:namespace/name".
	AttrCAFrom = "ca.from"
)

// releaseKeys are the labels and annotations that name an installation.
var releaseKeys = []string{
	"app.kubernetes.io/instance", "app.kubernetes.io/part-of",
	helmReleaseName,
}

// deployedKinds are the kinds a release's marks link to their release
// (DeployedBy): the workloads a Helm release deploys.
var deployedKinds = []inventory.Kind{
	KindDeployment, KindStatefulSet, KindDaemonSet, KindCronJob,
}

// helmMarkKeys name the Helm release that installed an object: Helm's
// own annotation and the instance label charts set to the release name.
// part-of names an application, not a release, so it links nothing.
var helmMarkKeys = []string{helmReleaseName, "app.kubernetes.io/instance"}

// annotateDeployer relates a workload to the Helm releases of its
// namespace its marks name. A release Helm records in another namespace
// (meta.helm.sh/release-namespace) is left out: only a release of the
// workload's own namespace is linked.
func annotateDeployer(obj metav1.Object, desc *Description) {
	if !slices.Contains(deployedKinds, desc.ID.Kind) {
		return
	}
	labels, annotations := obj.GetLabels(), obj.GetAnnotations()
	space := strings.TrimSpace(annotations[helmReleaseSpace])
	var targets []inventory.EntityID
	for _, key := range helmMarkKeys {
		value := strings.TrimSpace(labels[key])
		if value == "" {
			value = strings.TrimSpace(annotations[key])
		}
		if key == helmReleaseName && space != "" &&
			space != obj.GetNamespace() {
			continue
		}
		id := HelmReleaseID(obj.GetNamespace(),
			boundedLabel(value, maxOwnerValue))
		if id.Name != "" && !slices.Contains(targets, id) {
			targets = append(targets, id)
		}
	}
	if len(targets) == 0 {
		return
	}
	if desc.Relations == nil {
		desc.Relations = map[inventory.RelationType][]inventory.EntityID{}
	}
	desc.Relations[inventory.DeployedBy] = targets
}

// withDeployer adds DeployedBy to a schema's relation types when either
// version of the object has it. Only objects with release marks tell
// the relation, so the observations of every other object stay as they
// were; one whose marks went away clears it.
func withDeployer(
	types []inventory.RelationType, desc Description,
	previous *Description,
) []inventory.RelationType {
	had := previous != nil &&
		len(previous.Relations[inventory.DeployedBy]) > 0
	if (len(desc.Relations[inventory.DeployedBy]) == 0 && !had) ||
		slices.Contains(types, inventory.DeployedBy) {
		return types
	}
	return append(slices.Clip(types), inventory.DeployedBy)
}

// cert-manager annotations.
const (
	certNameAnnotation   = "cert-manager.io/certificate-name"
	injectFromAnnotation = "cert-manager.io/inject-ca-from"
	injectSecretFrom     = "cert-manager.io/inject-ca-from-secret"
)

// annotateRelease records the installation names and the cert-manager
// links of obj.
func annotateRelease(obj metav1.Object, desc *Description) {
	labels, annotations := obj.GetLabels(), obj.GetAnnotations()
	var names []string
	for _, key := range releaseKeys {
		value := strings.TrimSpace(labels[key])
		if value == "" {
			value = strings.TrimSpace(annotations[key])
		}
		value = boundedLabel(value, maxOwnerValue)
		if value != "" && !slices.Contains(names, value) {
			names = append(names, value)
		}
	}
	if len(names) > 0 {
		setAttribute(desc, AttrRelease,
			inventory.Text(strings.Join(names, ",")))
	}
	if name := annotations[certNameAnnotation]; name != "" {
		setAttribute(desc, AttrCertificateName, inventory.Text(
			boundedLabel(obj.GetNamespace()+"/"+name, 2*maxOwnerValue)))
	}
	from := "certificate:" + annotations[injectFromAnnotation]
	if secret := annotations[injectSecretFrom]; secret != "" {
		from = "secret:" + secret
	}
	if !strings.HasSuffix(from, ":") {
		setAttribute(desc, AttrCAFrom, inventory.Text(
			boundedLabel(from, 3*maxOwnerValue)))
	}
}
