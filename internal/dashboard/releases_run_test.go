package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/release"
	"github.com/abahmed/kwatch/internal/storage"
)

// A look finds a release in the model and saves the book; kwatch's own
// Deployment is left out.
func TestReleasesLookSavesTheBook(t *testing.T) {
	s := newTestServer()
	m := inventory.NewModel(inventory.Options{})
	for _, name := range []string{"api", "watcher"} {
		id := inventory.CoreID(kube.KindDeployment, "shop", name)
		m.Apply(inventory.Observation{Kind: inventory.Observed,
			Source: "test", At: testEpoch, Entity: id,
			Attributes: map[string]inventory.Value{
				kube.AttrReplicas: inventory.Number(1)}})
		m.Apply(inventory.Observation{Kind: inventory.Changed,
			Source: "test", At: testEpoch, Entity: id,
			Change: inventory.Change{Entity: id, At: testEpoch,
				Fields: []inventory.FieldChange{{
					Path: "containers[app].image", Before: name + ":1",
					After: name + ":2"}}}})
	}
	s.SetModel(m)
	store := openPushStore(t)
	values := storage.StateValues[release.Book](store)
	b := &releaseBook{}
	s.lookAtReleases(b, &values,
		OwnWorkload{Namespace: "shop", Name: "watcher"})
	saved, ok, err := values.Get(releasesKey)
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, saved.Records, 1)
	assert.Equal(t, "api", saved.Records[0].Name)
	assert.Equal(t, release.Rolling, saved.Records[0].Verdict)
}
