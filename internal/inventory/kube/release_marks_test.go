package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestReleaseNamesComeFromInstancePartOfAndHelm(t *testing.T) {
	translator := kube.NewTranslator(kube.PodSchema{})
	pod := maintenancePod(map[string]string{
		"meta.helm.sh/release-name":        "policy",
		"cert-manager.io/inject-ca-from":   "ops/policy-ca",
		"cert-manager.io/certificate-name": "serving"})
	pod.Labels = map[string]string{
		"app.kubernetes.io/instance": "policy",
		"app.kubernetes.io/part-of":  "policy-suite",
	}

	attrs := podAttributes(translator.Added(pod, true, ackAt))

	assert.Equal(t, "policy,policy-suite", attrs[kube.AttrRelease].AsText())
	assert.Equal(t, "certificate:ops/policy-ca",
		attrs[kube.AttrCAFrom].AsText())
	assert.Equal(t, pod.Namespace+"/serving",
		attrs[kube.AttrCertificateName].AsText())
}

func TestReleaseMarksAreAbsentWithoutTheirKeys(t *testing.T) {
	translator := kube.NewTranslator(kube.PodSchema{})
	pod := maintenancePod(map[string]string{
		"cert-manager.io/inject-ca-from-secret": "ops/ca"})
	pod.Labels = map[string]string{"app": "web"}

	attrs := podAttributes(translator.Added(pod, true, ackAt))

	assert.NotContains(t, attrs, kube.AttrRelease)
	assert.NotContains(t, attrs, kube.AttrCertificateName)
	assert.Equal(t, "secret:ops/ca", attrs[kube.AttrCAFrom].AsText())
}
