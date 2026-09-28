package knowledge

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewEntityID(t *testing.T) {
	id := NewEntityID("pod", "default", "my-pod")
	assert.Equal(t, Kind("pod"), id.Kind)
	assert.Equal(t, "default", id.Namespace)
	assert.Equal(t, "my-pod", id.Name)
}

func TestEntityIDString(t *testing.T) {
	tests := []struct {
		name     string
		id       EntityID
		expected string
	}{
		{
			"namespaced entity",
			EntityID{Kind: "pod", Namespace: "default", Name: "my-pod"},
			"pod/default/my-pod",
		},
		{
			"cluster-scoped entity (empty namespace)",
			EntityID{Kind: "node", Namespace: "", Name: "node-1"},
			"node//node-1",
		},
		{
			"with slashes in names",
			EntityID{Kind: "custom", Namespace: "ns", Name: "name"},
			"custom/ns/name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.id.String())
		})
	}
}

func TestParseEntityIDRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		id   EntityID
	}{
		{
			"namespaced",
			EntityID{Kind: "pod", Namespace: "default", Name: "my-pod"},
		},
		{
			"cluster-scoped",
			EntityID{Kind: "node", Namespace: "", Name: "node-1"},
		},
		{
			"empty namespace with pod",
			EntityID{Kind: "pod", Namespace: "", Name: "my-pod"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := tt.id.String()
			parsed, ok := ParseEntityID(key)
			assert.True(t, ok)
			assert.Equal(t, tt.id, parsed)
		})
	}
}

func TestParseEntityIDInvalid(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"empty string", ""},
		{"no slashes", "poddefaultmy-pod"},
		{"only one slash", "pod/default"},
		{"two slashes, no name", "pod/default/"},
		{"empty kind", "/default/my-pod"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := ParseEntityID(tt.key)
			assert.False(t, ok, "expected ParseEntityID to fail for %q", tt.key)
		})
	}
}

func TestParseEntityIDValidClusterScoped(t *testing.T) {
	// cluster-scoped entities have valid empty namespace
	id, ok := ParseEntityID("node//node-1")
	assert.True(t, ok)
	assert.Equal(t, Kind("node"), id.Kind)
	assert.Equal(t, "", id.Namespace)
	assert.Equal(t, "node-1", id.Name)
}

func TestEntityIDIsZero(t *testing.T) {
	tests := []struct {
		name     string
		id       EntityID
		expected bool
	}{
		{"zero value", EntityID{}, true},
		{"only kind set", EntityID{Kind: "pod"}, false},
		{"only name set", EntityID{Name: "my-pod"}, false},
		{"only namespace set", EntityID{Namespace: "default"}, true},
		{
			"namespaced entity",
			EntityID{Kind: "pod", Namespace: "default", Name: "my-pod"},
			false,
		},
		{
			"cluster-scoped entity",
			EntityID{Kind: "node", Namespace: "", Name: "node-1"},
			false,
		},
		{
			"empty namespace ok if kind and name set",
			EntityID{Kind: "node", Name: "node-1"},
			false,
		},
		{"empty kind and name", EntityID{Namespace: "default"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.id.IsZero())
		})
	}
}

func TestEntityClone(t *testing.T) {
	original := Entity{
		ID:  EntityID{Kind: "pod", Namespace: "default", Name: "my-pod"},
		UID: "uid-12345",
		Attributes: map[string]Attribute{
			"status": {Value: Text("running")},
		},
	}

	cloned := original.clone()

	// Verify contents match
	assert.Equal(t, original.ID, cloned.ID)
	assert.Equal(t, original.UID, cloned.UID)
	assert.Equal(t, original.Attributes, cloned.Attributes)

	// Verify independence: mutating cloned.Attributes doesn't affect original
	cloned.Attributes["status"] = Attribute{Value: Text("terminated")}
	assert.Equal(t, "running", original.Attributes["status"].Value.AsText())
	assert.Equal(t, "terminated", cloned.Attributes["status"].Value.AsText())

	// Verify new map was created
	cloned.Attributes["new"] = Attribute{Value: Text("added")}
	_, exists := original.Attributes["new"]
	assert.False(t, exists)
}

func TestAttributeAccess(t *testing.T) {
	e := Entity{
		ID: EntityID{Kind: "pod", Namespace: "default", Name: "my-pod"},
		Attributes: map[string]Attribute{
			"status": {Value: Text("running")},
		},
	}

	attr, ok := e.Attribute("status")
	assert.True(t, ok)
	assert.Equal(t, "running", attr.Value.AsText())

	_, ok = e.Attribute("missing")
	assert.False(t, ok)
}

func TestEntityIDEquality(t *testing.T) {
	id1 := NewEntityID("pod", "default", "my-pod")
	id2 := NewEntityID("pod", "default", "my-pod")
	id3 := NewEntityID("pod", "default", "other-pod")

	assert.Equal(t, id1, id2)
	assert.NotEqual(t, id1, id3)
}
