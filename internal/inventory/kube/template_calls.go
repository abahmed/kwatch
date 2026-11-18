package kube

import (
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrTemplateCalls lists what a workload's pod template is configured to
// call: the in-cluster Services and external endpoints its environment
// names, read as a pod's would be (podServiceCalls, podDependencies), as
// sorted entity keys joined by commas. It changes only when the template
// does, never as pods come and go, so the end of a call can be recorded
// on the workload (see LinkJournal). Absent when the template calls
// nothing.
const AttrTemplateCalls = "template.calls"

// templateCalls is the targets AttrTemplateCalls lists for a template in
// namespace.
func templateCalls(
	namespace string, template *corev1.PodTemplateSpec,
) []inventory.EntityID {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace},
		Spec:       template.Spec,
	}
	out := serviceCallIDs(podServiceCalls(pod))
	return append(out, podDependencies(pod)...)
}

// setTemplateCalls sets AttrTemplateCalls on attrs for the template.
func setTemplateCalls(
	attrs map[string]inventory.Value, namespace string,
	template *corev1.PodTemplateSpec,
) {
	targets := templateCalls(namespace, template)
	if len(targets) == 0 {
		return
	}
	keys := make([]string, len(targets))
	for i, target := range targets {
		keys[i] = target.String()
	}
	sort.Strings(keys)
	attrs[AttrTemplateCalls] = inventory.Text(strings.Join(keys, ","))
}

// workloadCalls are the calls the workload's template holds now, as
// links to record when they end.
func workloadCalls(r inventory.Reader, id inventory.EntityID) []heldLink {
	attribute, ok := r.Attribute(id, AttrTemplateCalls)
	if !ok {
		return nil
	}
	var out []heldLink
	for _, key := range strings.Split(attribute.Value.AsText(), ",") {
		if to, ok := inventory.ParseEntityID(key); ok {
			out = append(out, heldLink{from: id, rel: inventory.Calls, to: to})
		}
	}
	return out
}
