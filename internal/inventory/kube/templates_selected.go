package kube

import (
	"k8s.io/apimachinery/pkg/labels"

	"github.com/abahmed/kwatch/internal/inventory"
)

// templateKinds are the kinds whose pod template labels say which pods
// a Service may select once they run.
var templateKinds = append(
	[]inventory.Kind{KindReplicaSet, KindJob}, WorkloadKinds...)

// TemplateKinds returns the kinds SelectedByTemplate reads, so a caller
// can check that every one of them is synced before it concludes that
// no workload matches.
func TemplateKinds() []inventory.Kind {
	return append([]inventory.Kind(nil), templateKinds...)
}

// SelectedByTemplate reports whether any pod template in the namespace
// of a Service (or of another object with a pod selector, such as a
// disruption budget) carries labels its selector matches. It is the
// inverse of ServicesSelecting: it holds for a workload scaled to
// zero, which has no pod now but is the Service's backend when it runs.
func SelectedByTemplate(
	r inventory.Reader, service inventory.Entity,
) bool {
	selector := entityText(service, AttrSelector)
	if service.ID.Namespace == "" {
		return false
	}
	for _, kind := range templateKinds {
		for _, id := range r.EntitiesIn(kind, service.ID.Namespace) {
			template, ok := presentAttribute(r, id, AttrTemplateLabels)
			if !ok {
				continue
			}
			set, err := labels.ConvertSelectorToLabelsMap(
				template.Value.AsText())
			if err == nil && selectorMatches(selector, set) {
				return true
			}
		}
	}
	return false
}
