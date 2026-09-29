package kube

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// PodSchema describes Pods and their containers.
type PodSchema struct{}

// Kind implements Schema.
func (PodSchema) Kind() knowledge.Kind { return KindPod }

// RelationTypes implements Schema.
func (PodSchema) RelationTypes() []knowledge.RelationType {
	return []knowledge.RelationType{
		knowledge.OwnedBy, knowledge.RunsOn, knowledge.References,
		knowledge.Mounts, knowledge.Pulls,
	}
}

// Describe implements Schema.
func (PodSchema) Describe(obj any) (Description, bool) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return Description{}, false
	}
	id := objectID(KindPod, pod)
	rel := relations{}
	rel.add(knowledge.OwnedBy, ownerIDs(pod)...)
	rel.add(knowledge.RunsOn,
		knowledge.NewEntityID(KindNode, "", pod.Spec.NodeName))
	rel.add(knowledge.References, podReferences(pod)...)
	rel.add(knowledge.Mounts, podClaims(pod)...)
	rel.add(knowledge.Pulls, podImages(pod)...)
	return Description{
		ID: id, UID: string(pod.UID),
		Attributes: podAttributes(pod),
		Relations:  rel,
		Children:   containerDescriptions(pod, id),
	}, true
}

// Diff implements Schema. Pod specs are nearly immutable; deletion and
// resize requests are the meaningful changes.
func (PodSchema) Diff(old, new any) []knowledge.FieldChange {
	before, ok1 := old.(*corev1.Pod)
	after, ok2 := new.(*corev1.Pod)
	if !ok1 || !ok2 {
		return nil
	}
	var fields []knowledge.FieldChange
	if before.DeletionTimestamp == nil && after.DeletionTimestamp != nil {
		fields = append(fields, knowledge.FieldChange{
			Path: "metadata.deletionTimestamp", After: "set",
		})
	}
	fields = append(fields, resourceDiff(before, after)...)
	return fields
}

func podAttributes(pod *corev1.Pod) map[string]knowledge.Value {
	attrs := map[string]knowledge.Value{
		AttrPhase:    knowledge.Text(string(pod.Status.Phase)),
		AttrDeleting: knowledge.Bool(pod.DeletionTimestamp != nil),
		AttrQoS:      knowledge.Text(string(pod.Status.QOSClass)),
		AttrLabels:   knowledge.Text(labelText(pod.Labels)),
	}
	if pod.Status.Reason != "" {
		attrs[AttrReason] = knowledge.Text(pod.Status.Reason)
	}
	if pod.Status.Message != "" {
		attrs[AttrMessage] = knowledge.Text(truncate(pod.Status.Message))
	}
	if pod.Status.StartTime != nil {
		attrs[AttrStartTime] = knowledge.Time(pod.Status.StartTime.Time)
	}
	conditions := make([]condition, 0, len(pod.Status.Conditions))
	for _, c := range pod.Status.Conditions {
		conditions = append(conditions, condition{
			Type: string(c.Type), Status: string(c.Status),
			Reason: c.Reason, Message: c.Message,
			Since: c.LastTransitionTime.Time,
		})
		if c.Type == corev1.PodReady {
			attrs[AttrReady] = knowledge.Bool(
				c.Status == corev1.ConditionTrue,
			)
			attrs[AttrReadySince] = knowledge.Time(c.LastTransitionTime.Time)
		}
	}
	setConditions(attrs, conditions)
	return attrs
}

func podReferences(pod *corev1.Pod) []knowledge.EntityID {
	ns := pod.Namespace
	var out []knowledge.EntityID
	secret := func(name string) {
		out = append(out, knowledge.NewEntityID(KindSecret, ns, name))
	}
	configMap := func(name string) {
		out = append(out, knowledge.NewEntityID(KindConfigMap, ns, name))
	}
	for _, ref := range pod.Spec.ImagePullSecrets {
		secret(ref.Name)
	}
	for _, volume := range pod.Spec.Volumes {
		volumeReferences(volume, secret, configMap)
	}
	forEachContainer(pod, func(c corev1.Container, _ bool) {
		containerReferences(c, secret, configMap)
	})
	if account := pod.Spec.ServiceAccountName; account != "" {
		out = append(out, knowledge.NewEntityID(KindAccount, ns, account))
	}
	if name := pod.Spec.PriorityClassName; name != "" {
		out = append(out, knowledge.NewEntityID(KindPriorityClass, "", name))
	}
	if pod.Spec.RuntimeClassName != nil {
		out = append(out, knowledge.NewEntityID(
			KindRuntimeClass, "", *pod.Spec.RuntimeClassName,
		))
	}
	return out
}

func volumeReferences(
	volume corev1.Volume, secret, configMap func(string),
) {
	switch {
	case volume.Secret != nil:
		secret(volume.Secret.SecretName)
	case volume.ConfigMap != nil:
		configMap(volume.ConfigMap.Name)
	case volume.Projected != nil:
		for _, source := range volume.Projected.Sources {
			if source.Secret != nil {
				secret(source.Secret.Name)
			}
			if source.ConfigMap != nil {
				configMap(source.ConfigMap.Name)
			}
		}
	}
}

func containerReferences(
	c corev1.Container, secret, configMap func(string),
) {
	for _, from := range c.EnvFrom {
		if from.SecretRef != nil {
			secret(from.SecretRef.Name)
		}
		if from.ConfigMapRef != nil {
			configMap(from.ConfigMapRef.Name)
		}
	}
	for _, env := range c.Env {
		if env.ValueFrom == nil {
			continue
		}
		if ref := env.ValueFrom.SecretKeyRef; ref != nil {
			secret(ref.Name)
		}
		if ref := env.ValueFrom.ConfigMapKeyRef; ref != nil {
			configMap(ref.Name)
		}
	}
}

func podClaims(pod *corev1.Pod) []knowledge.EntityID {
	var out []knowledge.EntityID
	for _, volume := range pod.Spec.Volumes {
		if claim := volume.PersistentVolumeClaim; claim != nil {
			out = append(out, knowledge.NewEntityID(
				KindPVC, pod.Namespace, claim.ClaimName,
			))
		}
	}
	return out
}

func podImages(pod *corev1.Pod) []knowledge.EntityID {
	var out []knowledge.EntityID
	forEachContainer(pod, func(c corev1.Container, _ bool) {
		out = append(out, knowledge.NewEntityID(KindImage, "", c.Image))
	})
	return out
}

// forEachContainer visits init containers first, then regular ones.
func forEachContainer(pod *corev1.Pod, visit func(corev1.Container, bool)) {
	for _, c := range pod.Spec.InitContainers {
		visit(c, true)
	}
	for _, c := range pod.Spec.Containers {
		visit(c, false)
	}
}
