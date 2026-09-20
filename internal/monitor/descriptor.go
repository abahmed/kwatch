package monitor

import (
	"fmt"
	"sort"

	"github.com/abahmed/kwatch/internal/feature"
)

// Descriptor describes one user-visible monitoring capability.
//
// Resources uses Kwatch's lower-case singular resource vocabulary, while
// Feature is the stable catalog identifier used by configuration and docs.
// Documentation is a website-relative path, not a filesystem path.
type Descriptor struct {
	Name          string
	Description   string
	Feature       feature.ID
	Resources     []string
	Documentation string
}

// Registry stores monitor descriptors by name. It is metadata, not a service
// locator: application startup still constructs concrete monitors explicitly.
type Registry struct {
	descriptors map[string]Descriptor
}

// NewRegistry creates a validated registry from the supplied descriptors.
func NewRegistry(descriptors ...Descriptor) (*Registry, error) {
	r := &Registry{descriptors: make(map[string]Descriptor)}
	for _, descriptor := range descriptors {
		if err := r.Register(descriptor); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Register adds one descriptor and rejects ambiguous metadata.
func (r *Registry) Register(descriptor Descriptor) error {
	if r == nil {
		return fmt.Errorf("monitor registry is nil")
	}
	if r.descriptors == nil {
		r.descriptors = make(map[string]Descriptor)
	}
	if descriptor.Name == "" {
		return fmt.Errorf("monitor descriptor has an empty name")
	}
	if descriptor.Description == "" {
		return fmt.Errorf("monitor %q has an empty description", descriptor.Name)
	}
	if descriptor.Feature == "" {
		return fmt.Errorf("monitor %q has no feature id", descriptor.Name)
	}
	if _, ok := feature.Lookup(descriptor.Feature); !ok {
		return fmt.Errorf(
			"monitor %q references unknown feature %q",
			descriptor.Name,
			descriptor.Feature,
		)
	}
	if _, exists := r.descriptors[descriptor.Name]; exists {
		return fmt.Errorf("monitor %q is registered more than once", descriptor.Name)
	}
	copyDescriptor := descriptor
	copyDescriptor.Resources = append([]string(nil), descriptor.Resources...)
	r.descriptors[descriptor.Name] = copyDescriptor
	return nil
}

// Lookup returns a copy of a descriptor by name.
func (r *Registry) Lookup(name string) (Descriptor, bool) {
	if r == nil {
		return Descriptor{}, false
	}
	descriptor, ok := r.descriptors[name]
	if !ok {
		return Descriptor{}, false
	}
	descriptor.Resources = append([]string(nil), descriptor.Resources...)
	return descriptor, true
}

// List returns descriptors in stable name order.
func (r *Registry) List() []Descriptor {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.descriptors))
	for name := range r.descriptors {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]Descriptor, 0, len(names))
	for _, name := range names {
		descriptor := r.descriptors[name]
		descriptor.Resources = append(
			[]string(nil), descriptor.Resources...,
		)
		result = append(result, descriptor)
	}
	return result
}
