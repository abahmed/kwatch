package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func controllerDeployment(
	annotations map[string]string, container corev1.Container,
) *appsv1.Deployment {
	d := deployment("ctl")
	d.Spec.Template.Annotations = annotations
	d.Spec.Template.Spec.Containers = []corev1.Container{container}
	return d
}

func templateAttributes(
	t *testing.T, d *appsv1.Deployment,
) map[string]inventory.Value {
	t.Helper()
	desc, ok := kube.DeploymentSchema().Describe(d)
	require.True(t, ok)
	return desc.Attributes
}

func TestWorkloadRecordsTheDomainsOfItsPodAnnotations(t *testing.T) {
	d := controllerDeployment(map[string]string{
		"widgets.example.com/owner": "x", "Gadgets.Example.com/y": "z",
		"kubernetes.io/change-cause": "c", "plain": "p",
		"cluster-autoscaler.kubernetes.io/safe": "true"},
		corev1.Container{Name: "app"})

	attrs := templateAttributes(t, d)

	assert.Equal(t, "gadgets.example.com widgets.example.com",
		attrs[kube.AttrTemplateAnnotationDomains].AsText())
}

// Arguments and literal environment values may quote the API group or
// finalizer the controller serves; nothing named like a credential is
// read, and an address or a version is not a name.
func TestWorkloadRecordsTheNamesItsContainersQuote(t *testing.T) {
	d := controllerDeployment(nil, corev1.Container{Name: "app",
		Args: []string{
			"--watch-group=widgets.example.com",
			"--finalizer", "widgets.example.com/cleanup",
			"--bind=0.0.0.0:8080", "--version=1.2.3",
			"--api-token=leak.example.com", "--secret", "hidden.example.com",
			"--after=shown.example.com"},
		Env: []corev1.EnvVar{
			{Name: "GROUP", Value: "gadgets.example.com"},
			{Name: "GITHUB_TOKEN", Value: "tok.example.com"},
			{Name: "FROM_SECRET", ValueFrom: &corev1.EnvVarSource{}}}})

	attrs := templateAttributes(t, d)

	assert.Equal(t,
		"widgets.example.com widgets.example.com/cleanup "+
			"shown.example.com gadgets.example.com",
		attrs[kube.AttrTemplateQuoted].AsText())
}

func TestWorkloadWithoutSignalsHasNoSignalAttributes(t *testing.T) {
	attrs := templateAttributes(t, deployment("plain"))

	assert.NotContains(t, attrs, kube.AttrTemplateQuoted)
	assert.NotContains(t, attrs, kube.AttrTemplateAnnotationDomains)
}

// A connection URL carries its password as "user:password@host". The
// password may look like a qualified name; it must never be kept.
func TestWorkloadQuotedNamesNeverKeepURLCredentials(t *testing.T) {
	d := controllerDeployment(nil, corev1.Container{Name: "app",
		Args: []string{"--dsn=mysql://root:hunter2.pw@db.example.com/x"},
		Env: []corev1.EnvVar{
			{Name: "DATABASE_URL",
				Value: "postgres://app:s3cr3t.pw@db.example.com/x"},
			{Name: "PLAIN", Value: "https://gadgets.example.com/ok"},
			{Name: "CRED_ONLY", Value: "ftp://user.name@files.example.com"}}})

	attrs := templateAttributes(t, d)

	quoted := attrs[kube.AttrTemplateQuoted].AsText()
	assert.NotContains(t, quoted, "s3cr3t")
	assert.NotContains(t, quoted, "hunter2")
	assert.NotContains(t, quoted, "user.name")
	assert.Contains(t, quoted, "db.example.com")
	assert.Contains(t, quoted, "gadgets.example.com")
}
