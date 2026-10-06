package kube

import (
	"k8s.io/apimachinery/pkg/labels"

	"github.com/abahmed/kwatch/internal/inventory"
)

// ServicesSelecting returns the Services in the workload's namespace
// whose selector matches the labels of its pod template. Unlike the
// Selects links, which go through the pods that exist, it works for a
// workload with no pods at all, such as one scaled to zero.
func ServicesSelecting(
	r inventory.Reader, workload inventory.Entity,
) []inventory.EntityID {
	podLabels, err := labels.ConvertSelectorToLabelsMap(
		entityText(workload, AttrTemplateLabels))
	if err != nil || len(podLabels) == 0 || workload.ID.Namespace == "" {
		return nil
	}
	var out []inventory.EntityID
	for _, id := range r.EntitiesIn(KindService, workload.ID.Namespace) {
		service, ok := r.Entity(id)
		if ok && selectorMatches(entityText(service, AttrSelector),
			podLabels) {
			out = append(out, id)
		}
	}
	return out
}

// selectorMatches reports a non-empty Service selector that matches
// the labels. A Service with no selector has no pods of its own.
func selectorMatches(text string, set labels.Set) bool {
	if text == "" || text == "<none>" {
		return false
	}
	selector, err := labels.Parse(text)
	return err == nil && selector.Matches(set)
}
