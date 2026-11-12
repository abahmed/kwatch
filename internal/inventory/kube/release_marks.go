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
