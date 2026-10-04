package kube

import (
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrOptionalRefs lists the Secrets and ConfigMaps a pod references only
// as optional, as sorted "kind/name" entries joined by commas. The
// references relation still links them, so changes to them reach their
// consumers, but their absence does not break the pod.
const AttrOptionalRefs = "references.optional"

// OptionalReference reports whether pod references ref only as optional.
func OptionalReference(pod inventory.Entity, ref inventory.EntityID) bool {
	attribute, ok := pod.Attribute(AttrOptionalRefs)
	if !ok {
		return false
	}
	want := string(ref.Kind) + "/" + ref.Name
	for _, entry := range strings.Split(attribute.Value.AsText(), ",") {
		if entry == want {
			return true
		}
	}
	return false
}

// PodSchema describes Pods and their containers.
type PodSchema struct{}

// Kind implements Schema.
func (PodSchema) Kind() inventory.Kind { return KindPod }

// RelationTypes implements Schema.
func (PodSchema) RelationTypes() []inventory.RelationType {
	return []inventory.RelationType{
		inventory.OwnedBy, inventory.RunsOn, inventory.References,
		inventory.Mounts, inventory.Pulls, inventory.Calls,
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
	rel.add(inventory.OwnedBy, ownerIDs(pod)...)
	rel.add(inventory.RunsOn,
		inventory.CoreID(KindNode, "", pod.Spec.NodeName))
	rel.add(inventory.References, podReferences(pod)...)
	rel.add(inventory.Mounts, podClaims(pod)...)
	rel.add(inventory.Pulls, podImages(pod)...)
	rel.add(inventory.Calls, podDependencies(pod)...)
	return Description{
		ID: id, UID: string(pod.UID),
		Attributes: podAttributes(pod),
		Relations:  rel,
		Children:   containerDescriptions(pod, id),
	}, true
}

// Diff implements Schema. Pod specs are nearly immutable; deletion,
// resize requests and label edits (which decide what selects the pod)
// are the meaningful changes.
func (PodSchema) Diff(old, new any) []inventory.FieldChange {
	before, ok1 := old.(*corev1.Pod)
	after, ok2 := new.(*corev1.Pod)
	if !ok1 || !ok2 {
		return nil
	}
	var fields []inventory.FieldChange
	if before.DeletionTimestamp == nil && after.DeletionTimestamp != nil {
		fields = append(fields, inventory.FieldChange{
			Path: "metadata.deletionTimestamp", After: "set",
		})
	}
	fields = append(fields, resourceDiff(before, after)...)
	if b, a := labelText(before.Labels), labelText(after.Labels); b != a {
		fields = append(fields, inventory.FieldChange{
			Path: "metadata.labels", Before: b, After: a,
		})
	}
	return fields
}

func podAttributes(pod *corev1.Pod) map[string]inventory.Value {
	attrs := map[string]inventory.Value{
		AttrPhase:    inventory.Text(string(pod.Status.Phase)),
		AttrDeleting: inventory.Bool(pod.DeletionTimestamp != nil),
		AttrQoS:      inventory.Text(string(pod.Status.QOSClass)),
		AttrLabels:   inventory.Text(labelText(pod.Labels)),
	}
	if pod.Status.Reason != "" {
		attrs[AttrReason] = inventory.Text(pod.Status.Reason)
	}
	if pod.Status.Message != "" {
		attrs[AttrMessage] = inventory.Text(evidenceText(pod.Status.Message))
	}
	if pod.Status.StartTime != nil {
		attrs[AttrStartTime] = inventory.Time(pod.Status.StartTime.Time)
	}
	if pod.Status.PodIP != "" {
		attrs[AttrPodIP] = inventory.Text(pod.Status.PodIP)
	}
	if !pod.CreationTimestamp.IsZero() {
		attrs[AttrCreated] = inventory.Time(pod.CreationTimestamp.Time)
	}
	conditions := make([]condition, 0, len(pod.Status.Conditions))
	for _, c := range pod.Status.Conditions {
		conditions = append(conditions, condition{
			Type: string(c.Type), Status: string(c.Status),
			Reason: c.Reason, Message: c.Message,
			Since: c.LastTransitionTime.Time,
		})
		if c.Type == corev1.PodReady {
			attrs[AttrReady] = inventory.Bool(
				c.Status == corev1.ConditionTrue,
			)
			attrs[AttrReadySince] = inventory.Time(c.LastTransitionTime.Time)
		}
	}
	setConditions(attrs, conditions)
	if optional := optionalReferences(pod); optional != "" {
		attrs[AttrOptionalRefs] = inventory.Text(optional)
	}
	return attrs
}

// podRef is one Secret or ConfigMap reference and whether the pod starts
// without the object.
type podRef struct {
	kind     inventory.Kind
	name     string
	optional bool
}

// refSink collects references; nil optional flags mean required.
type refSink func(kind inventory.Kind, name string, optional *bool)

func podReferences(pod *corev1.Pod) []inventory.EntityID {
	ns := pod.Namespace
	var out []inventory.EntityID
	for _, ref := range configReferences(pod) {
		out = append(out, inventory.CoreID(ref.kind, ns, ref.name))
	}
	if account := pod.Spec.ServiceAccountName; account != "" {
		out = append(out, inventory.CoreID(KindAccount, ns, account))
	}
	if name := pod.Spec.PriorityClassName; name != "" {
		out = append(out, inventory.CoreID(KindPriorityClass, "", name))
	}
	if pod.Spec.RuntimeClassName != nil {
		out = append(out, inventory.CoreID(
			KindRuntimeClass, "", *pod.Spec.RuntimeClassName,
		))
	}
	return out
}

// configReferences lists the pod's Secret and ConfigMap references in
// spec order. Image pull secrets are required: kubelet cannot pull a
// private image without them.
func configReferences(pod *corev1.Pod) []podRef {
	var out []podRef
	add := func(kind inventory.Kind, name string, optional *bool) {
		out = append(out, podRef{
			kind: kind, name: name,
			optional: optional != nil && *optional,
		})
	}
	for _, ref := range pod.Spec.ImagePullSecrets {
		add(KindSecret, ref.Name, nil)
	}
	for _, volume := range pod.Spec.Volumes {
		volumeReferences(volume, add)
	}
	forEachContainer(pod, func(c corev1.Container, _ bool) {
		containerReferences(c, add)
	})
	return out
}

// optionalReferences lists the references that are optional everywhere
// the pod uses them; one required use makes the object required.
func optionalReferences(pod *corev1.Pod) string {
	required := map[string]bool{}
	optional := map[string]bool{}
	for _, ref := range configReferences(pod) {
		key := string(ref.kind) + "/" + ref.name
		if ref.optional {
			optional[key] = true
		} else {
			required[key] = true
		}
	}
	var out []string
	for key := range optional {
		if !required[key] {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func volumeReferences(volume corev1.Volume, add refSink) {
	switch {
	case volume.Secret != nil:
		add(KindSecret, volume.Secret.SecretName, volume.Secret.Optional)
	case volume.ConfigMap != nil:
		add(KindConfigMap, volume.ConfigMap.Name, volume.ConfigMap.Optional)
	case volume.Projected != nil:
		for _, source := range volume.Projected.Sources {
			if source.Secret != nil {
				add(KindSecret, source.Secret.Name, source.Secret.Optional)
			}
			if source.ConfigMap != nil {
				add(KindConfigMap, source.ConfigMap.Name,
					source.ConfigMap.Optional)
			}
		}
	}
}

func containerReferences(c corev1.Container, add refSink) {
	for _, from := range c.EnvFrom {
		if ref := from.SecretRef; ref != nil {
			add(KindSecret, ref.Name, ref.Optional)
		}
		if ref := from.ConfigMapRef; ref != nil {
			add(KindConfigMap, ref.Name, ref.Optional)
		}
	}
	for _, env := range c.Env {
		if env.ValueFrom == nil {
			continue
		}
		if ref := env.ValueFrom.SecretKeyRef; ref != nil {
			add(KindSecret, ref.Name, ref.Optional)
		}
		if ref := env.ValueFrom.ConfigMapKeyRef; ref != nil {
			add(KindConfigMap, ref.Name, ref.Optional)
		}
	}
}

func podClaims(pod *corev1.Pod) []inventory.EntityID {
	var out []inventory.EntityID
	for _, volume := range pod.Spec.Volumes {
		if claim := volume.PersistentVolumeClaim; claim != nil {
			out = append(out, inventory.CoreID(
				KindPVC, pod.Namespace, claim.ClaimName,
			))
		}
	}
	return out
}

func podImages(pod *corev1.Pod) []inventory.EntityID {
	var out []inventory.EntityID
	forEachContainer(pod, func(c corev1.Container, _ bool) {
		out = append(out, inventory.CoreID(KindImage, "", c.Image))
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
