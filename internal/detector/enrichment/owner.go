package enrichment

import (
	"github.com/abahmed/kwatch/internal/detector"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type OwnerEnricher struct{}

func NewOwnerEnricher() *OwnerEnricher {
	return &OwnerEnricher{}
}

func (e *OwnerEnricher) Name() string {
	return "OwnerEnricher"
}

func (e *OwnerEnricher) Enrich(input *detector.Input) error {
	if input.Pod == nil {
		return nil
	}

	owners := input.Pod.GetOwnerReferences()
	if len(owners) > 0 {
		input.Owner = &owners[0]
	}

	return nil
}

func GetOwnerKind(input *detector.Input) string {
	if input.Owner == nil {
		return ""
	}
	return string(input.Owner.Kind)
}

func GetOwnerName(input *detector.Input) string {
	if input.Owner == nil {
		return ""
	}
	return input.Owner.Name
}

func FindOwnerRef(pod *metav1.OwnerReference) *metav1.OwnerReference {
	return pod
}
