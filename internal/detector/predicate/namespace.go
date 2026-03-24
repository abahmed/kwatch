package predicate

import (
	"slices"

	"github.com/abahmed/kwatch/internal/detector"
)

// Namespace filters events by namespace
type Namespace struct {
	Allowed   []string
	Forbidden []string
}

func NewNamespace(allowed, forbidden []string) *Namespace {
	return &Namespace{
		Allowed:   allowed,
		Forbidden: forbidden,
	}
}

func (n *Namespace) Name() string {
	return "NamespacePredicate"
}

func (n *Namespace) Filter(input *detector.Input) bool {
	if input.Pod == nil {
		return false
	}

	namespace := input.Pod.Namespace

	if len(n.Allowed) > 0 && !slices.Contains(n.Allowed, namespace) {
		return true // filter out
	}

	if len(n.Forbidden) > 0 && slices.Contains(n.Forbidden, namespace) {
		return true // filter out
	}

	return false
}
