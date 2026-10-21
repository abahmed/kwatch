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

// SelectedByTemplate reports whether any pod template in the Service's
// namespace carries labels the Service's selector matches. It is the
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
			workload, ok := r.Entity(id)
			if !ok {
				continue
			}
			set, err := labels.ConvertSelectorToLabelsMap(
				entityText(workload, AttrTemplateLabels))
			if err == nil && selectorMatches(selector, set) {
				return true
			}
		}
	}
	return false
}
