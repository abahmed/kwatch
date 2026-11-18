package kube

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/format"
)

// Source annotations: the OCI image annotation keys, which CI and
// GitOps pipelines also set on the objects they deploy to say which
// repository and revision a release was built from. deployment.
// kubernetes.io/revision and app.kubernetes.io/version are not commits
// and are never read as one.
const (
	sourceRepoAnnotation     = "org.opencontainers.image.source"
	sourceRevisionAnnotation = "org.opencontainers.image.revision"
)

// changeSource reads the repository and revision recorded on obj, or on
// its pod template when the object itself has none. A value that is
// not a plain https repository address or a commit SHA or tag is
// dropped: it is never shown nor linked.
func changeSource(obj any) (repo, commit string) {
	for _, annotations := range sourceAnnotations(obj) {
		if repo == "" {
			repo, _ = format.RepoURL(annotations[sourceRepoAnnotation])
		}
		if commit == "" {
			value := annotations[sourceRevisionAnnotation]
			if format.ValidRevision(value) {
				commit = value
			}
		}
	}
	return repo, commit
}

// sourceAnnotations are the annotations of obj, then of its pod
// template when it has one.
func sourceAnnotations(obj any) []map[string]string {
	var out []map[string]string
	if meta, ok := obj.(metav1.Object); ok {
		out = append(out, meta.GetAnnotations())
	}
	if template := podTemplateOf(obj); template != nil {
		out = append(out, template.Annotations)
	}
	return out
}

// podTemplateOf is the pod template of a workload, or nil.
func podTemplateOf(obj any) *corev1.PodTemplateSpec {
	switch typed := obj.(type) {
	case *appsv1.Deployment:
		return &typed.Spec.Template
	case *appsv1.StatefulSet:
		return &typed.Spec.Template
	case *appsv1.DaemonSet:
		return &typed.Spec.Template
	case *appsv1.ReplicaSet:
		return &typed.Spec.Template
	case *batchv1.Job:
		return &typed.Spec.Template
	case *batchv1.CronJob:
		return &typed.Spec.JobTemplate.Spec.Template
	}
	return nil
}
