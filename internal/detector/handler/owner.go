package handler

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/detector"
)

// OwnerHandler enriches events with owner reference
type OwnerHandler struct {
	client kubernetes.Interface
}

func NewOwnerHandler(client kubernetes.Interface) *OwnerHandler {
	return &OwnerHandler{
		client: client,
	}
}

func (h *OwnerHandler) Name() string {
	return "OwnerHandler"
}

func (h *OwnerHandler) Handle(input *detector.Input) error {
	if input.Pod == nil {
		return nil
	}

	// Get owner references from pod
	owners := input.Pod.GetOwnerReferences()
	if len(owners) == 0 {
		return nil
	}

	// Get the first owner (usually the controller)
	for _, owner := range owners {
		if owner.Controller != nil && *owner.Controller {
			input.Owner = &owner
			break
		}
	}

	// If no controller, use first owner
	if input.Owner == nil && len(owners) > 0 {
		input.Owner = &owners[0]
	}

	return nil
}

// GetOwnerName returns the owner name
func GetOwnerName(owner *metav1.OwnerReference) string {
	if owner == nil {
		return ""
	}
	return owner.Name
}
