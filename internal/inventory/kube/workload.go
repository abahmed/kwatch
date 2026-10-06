package kube

import (
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// workloadSchema describes any controller that owns a pod template. The
// accessors keep kind-specific code to a few lines per kind.
type workloadSchema[T metav1.Object] struct {
	kind     inventory.Kind
	template func(T) *corev1.PodTemplateSpec
	replicas func(T) *int32
	status   func(T, map[string]inventory.Value)
	extra    func(old, new T) []inventory.FieldChange
}

// Kind implements Schema.
func (s workloadSchema[T]) Kind() inventory.Kind { return s.kind }

// RelationTypes implements Schema.
func (s workloadSchema[T]) RelationTypes() []inventory.RelationType {
	return []inventory.RelationType{inventory.OwnedBy, inventory.References}
}

// Describe implements Schema.
func (s workloadSchema[T]) Describe(obj any) (Description, bool) {
	workload, ok := obj.(T)
	if !ok {
		return Description{}, false
	}
	attrs := map[string]inventory.Value{
		AttrGeneration: inventory.Number(float64(workload.GetGeneration())),
		AttrDeleting:   inventory.Bool(workload.GetDeletionTimestamp() != nil),
		AttrCreated: inventory.Time(
			workload.GetCreationTimestamp().Time),
	}
	rel := relations{}
	rel.add(inventory.OwnedBy, ownerIDs(workload)...)
	if template := s.template(workload); template != nil {
		attrs[AttrTemplateHash] = inventory.Text(TemplateHash(template))
		attrs[AttrTemplateLabels] = inventory.Text(
			labelText(template.Labels))
		rel.add(inventory.References,
			templateReferences(workload.GetNamespace(), template)...)
	}
	if s.replicas != nil {
		if replicas := s.replicas(workload); replicas != nil {
			attrs[AttrReplicas] = inventory.Number(float64(*replicas))
		}
	}
	s.status(workload, attrs)
	return Description{
		ID: objectID(s.kind, workload), UID: string(workload.GetUID()),
		Attributes: attrs, Relations: rel,
	}, true
}

// Diff implements Schema: template changes (rollouts), replica changes and
// kind-specific spec edits. Status is never a change.
func (s workloadSchema[T]) Diff(old, new any) []inventory.FieldChange {
	before, ok1 := old.(T)
	after, ok2 := new.(T)
	if !ok1 || !ok2 {
		return nil
	}
	fields := templateDiff(s.template(before), s.template(after))
	if s.replicas != nil {
		b, a := s.replicas(before), s.replicas(after)
		if replicaText(b) != replicaText(a) {
			fields = append(fields, inventory.FieldChange{
				Path: "spec.replicas", Before: replicaText(b),
				After: replicaText(a),
			})
		}
	}
	if s.extra != nil {
		fields = append(fields, s.extra(before, after)...)
	}
	return fields
}

func replicaText(replicas *int32) string {
	if replicas == nil {
		return ""
	}
	return strconv.Itoa(int(*replicas))
}

// DeploymentSchema describes Deployments.
func DeploymentSchema() Schema {
	return workloadSchema[*appsv1.Deployment]{
		kind: KindDeployment,
		template: func(d *appsv1.Deployment) *corev1.PodTemplateSpec {
			return &d.Spec.Template
		},
		replicas: func(d *appsv1.Deployment) *int32 { return d.Spec.Replicas },
		status:   deploymentStatus,
		extra: func(b, a *appsv1.Deployment) []inventory.FieldChange {
			return boolChange("spec.paused", b.Spec.Paused, a.Spec.Paused)
		},
	}
}

// ReplicaSetSchema describes ReplicaSets.
func ReplicaSetSchema() Schema {
	return workloadSchema[*appsv1.ReplicaSet]{
		kind: KindReplicaSet,
		template: func(r *appsv1.ReplicaSet) *corev1.PodTemplateSpec {
			return &r.Spec.Template
		},
		replicas: func(r *appsv1.ReplicaSet) *int32 { return r.Spec.Replicas },
		status:   replicaSetStatus,
	}
}

// StatefulSetSchema describes StatefulSets.
func StatefulSetSchema() Schema {
	return workloadSchema[*appsv1.StatefulSet]{
		kind: KindStatefulSet,
		template: func(s *appsv1.StatefulSet) *corev1.PodTemplateSpec {
			return &s.Spec.Template
		},
		replicas: func(s *appsv1.StatefulSet) *int32 { return s.Spec.Replicas },
		status:   statefulSetStatus,
	}
}

// DaemonSetSchema describes DaemonSets.
func DaemonSetSchema() Schema {
	return workloadSchema[*appsv1.DaemonSet]{
		kind: KindDaemonSet,
		template: func(d *appsv1.DaemonSet) *corev1.PodTemplateSpec {
			return &d.Spec.Template
		},
		status: daemonSetStatus,
	}
}

// JobSchema describes Jobs.
func JobSchema() Schema {
	return workloadSchema[*batchv1.Job]{
		kind: KindJob,
		template: func(j *batchv1.Job) *corev1.PodTemplateSpec {
			return &j.Spec.Template
		},
		status: jobStatus,
		extra: func(b, a *batchv1.Job) []inventory.FieldChange {
			return boolChange("spec.suspend",
				b.Spec.Suspend != nil && *b.Spec.Suspend,
				a.Spec.Suspend != nil && *a.Spec.Suspend)
		},
	}
}

func boolChange(path string, before, after bool) []inventory.FieldChange {
	if before == after {
		return nil
	}
	return []inventory.FieldChange{{
		Path: path, Before: boolText(before), After: boolText(after),
	}}
}

// templateReferences lists what a pod template references in namespace.
func templateReferences(
	namespace string, template *corev1.PodTemplateSpec,
) []inventory.EntityID {
	return podReferences(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace},
		Spec:       template.Spec,
	})
}
