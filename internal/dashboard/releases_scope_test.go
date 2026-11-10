package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// released is a model with one Deployment per namespace, each with a
// new image.
func released(namespaces ...string) *inventory.Model {
	m := inventory.NewModel(inventory.Options{})
	for _, ns := range namespaces {
		id := inventory.CoreID(kube.KindDeployment, ns, "api")
		m.Apply(inventory.Observation{Kind: inventory.Observed,
			Source: "test", At: testEpoch, Entity: id,
			Attributes: map[string]inventory.Value{
				kube.AttrReplicas: inventory.Number(1)}})
		m.Apply(inventory.Observation{Kind: inventory.Changed,
			Source: "test", At: testEpoch, Entity: id,
			Change: inventory.Change{Entity: id, At: testEpoch,
				Fields: []inventory.FieldChange{{
					Path: "containers[app].image", Before: "api:1",
					After: "api:2"}}}})
	}
	return m
}

// The releases follow the whole namespace scope when it is given: a
// namespace the label selector leaves out has no releases, though the
// allowed and forbidden lists would let it in.
func TestReleasesLookFollowsTheWholeNamespaceScope(t *testing.T) {
	s := newTestServer()
	m := released("shop", "dev")
	s.SetModel(m)
	var asked inventory.Reader
	s.opts.Scope = Scope{Allowed: func(
		model inventory.Reader, namespace string,
	) bool {
		asked = model
		return namespace != "dev"
	}}
	b := &releaseBook{}
	s.lookAtReleases(b, nil, OwnWorkload{})
	book := b.snapshot()
	require.Len(t, book.Records, 1)
	assert.Equal(t, "shop", book.Records[0].Namespace)
	assert.NotNil(t, asked, "the scope reads the live model")
}

// Without it the two lists decide, as before.
func TestReleasesLookFollowsTheListsWithoutAScopeFunction(t *testing.T) {
	s := newTestServer()
	s.SetModel(released("shop", "dev"))
	s.opts.Scope = Scope{Excluded: []string{"dev"}}
	b := &releaseBook{}
	s.lookAtReleases(b, nil, OwnWorkload{})
	book := b.snapshot()
	require.Len(t, book.Records, 1)
	assert.Equal(t, "shop", book.Records[0].Namespace)
}
