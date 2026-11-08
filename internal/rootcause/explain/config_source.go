package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// configSource names the ConfigMap key the pods read an external
// endpoint's address from. It is nil for any other root and for an
// endpoint the pods name directly in their environment.
func (s Snapshot) configSource(c Cause) *rootcause.ConfigSource {
	if c.Root.Kind != KindExternalEndpoint || s.Model == nil {
		return nil
	}
	e, ok := s.Model.Entity(c.Root)
	if !ok {
		return nil
	}
	attribute, ok := e.Attribute(kube.AttrEndpointSources)
	if !ok {
		return nil
	}
	entries := strings.Split(attribute.Value.AsText(), ",")
	object, key, found := strings.Cut(entries[0], "#")
	namespace, name, placed := strings.Cut(object, "/")
	if !found || !placed || key == "" {
		return nil
	}
	return &rootcause.ConfigSource{
		ConfigMap: inventory.CoreID(kube.KindConfigMap, namespace, name),
		Key:       key, Address: c.Root.Name, More: len(entries) - 1,
	}
}
