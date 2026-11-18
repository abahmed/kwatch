package kube

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/inventory"
)

var helmT0 = time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)

func helmSecret(release string, version int, status string,
) *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{
		Namespace: "shop",
		Name: "sh.helm.release.v1." + release + ".v" +
			strconv.Itoa(version),
		CreationTimestamp: metav1.NewTime(helmT0.Add(
			time.Duration(version) * time.Minute)),
		Labels: map[string]string{
			"owner": "helm", "name": release, "status": status,
			"version": strconv.Itoa(version),
		},
	}}
}

func TestHelmRevisionOfReadsLabelsOnly(t *testing.T) {
	s := helmSecret("payments", 3, "failed")
	s.Labels["modifiedAt"] = strconv.FormatInt(helmT0.Unix(), 10)
	r, ok := HelmRevisionOf(s)
	require.True(t, ok)
	assert.Equal(t, HelmRevision{Namespace: "shop", Release: "payments",
		Status: "failed", Version: 3, Since: helmT0,
		Created: s.CreationTimestamp.Time}, r)

	other := helmSecret("payments", 3, "failed")
	other.Labels["owner"] = "someone"
	_, ok = HelmRevisionOf(other)
	assert.False(t, ok, "a Secret Helm did not write is ignored")
}

func TestHelmReleaseObservationUsesLatestRevision(t *testing.T) {
	id := HelmReleaseID("shop", "payments")
	var revisions []HelmRevision
	for i, status := range []string{"superseded", "deployed", "failed"} {
		r, _ := HelmRevisionOf(helmSecret("payments", i+1, status))
		revisions = append(revisions, r)
	}
	o := HelmReleaseObservation(id, revisions, helmT0)
	assert.Equal(t, inventory.Observed, o.Kind)
	assert.Equal(t, "failed", o.Attributes[AttrHelmStatus].AsText())
	rev, _ := o.Attributes[AttrHelmRevision].AsNumber()
	good, _ := o.Attributes[AttrHelmGoodRevision].AsNumber()
	assert.Equal(t, 3.0, rev)
	assert.Equal(t, 2.0, good)
	assert.True(t, revisions[2].Created.Equal(
		o.Attributes[AttrHelmRevisionCreated].AsTime()),
		"the latest revision began when its Secret was created")

	gone := HelmReleaseObservation(id, nil, helmT0)
	assert.Equal(t, inventory.Gone, gone.Kind)
}

func TestHelmHandlerDescribesTheWholeRelease(t *testing.T) {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc,
		cache.Indexers{helmIndex: helmIndexFunc})
	first := helmSecret("payments", 1, "deployed")
	second := helmSecret("payments", 2, "pending-upgrade")
	require.NoError(t, indexer.Add(first))
	require.NoError(t, indexer.Add(second))
	var got []inventory.Observation
	h := helmHandler(context.Background, indexer,
		func(_ context.Context, o ...inventory.Observation) {
			got = append(got, o...)
		}, func() time.Time { return helmT0 })

	h.OnAdd(second, false)
	require.Len(t, got, 1)
	assert.Equal(t, "pending-upgrade",
		got[0].Attributes[AttrHelmStatus].AsText())

	require.NoError(t, indexer.Delete(second))
	require.NoError(t, indexer.Delete(first))
	h.OnDelete(cache.DeletedFinalStateUnknown{Obj: first})
	require.Len(t, got, 2)
	assert.Equal(t, inventory.Gone, got[1].Kind)
}

func TestTrimHelmMetaDropsManagedFields(t *testing.T) {
	s := helmSecret("payments", 1, "deployed")
	s.ManagedFields = []metav1.ManagedFieldsEntry{{Manager: "helm"}}
	s.Annotations = map[string]string{"a": "b"}
	out, err := trimHelmMeta(s)
	require.NoError(t, err)
	trimmed := out.(*metav1.PartialObjectMetadata)
	assert.Nil(t, trimmed.ManagedFields)
	assert.Nil(t, trimmed.Annotations)
	assert.Equal(t, "payments", trimmed.Labels["name"])
}

func TestHelmReleasesFollowTheSecretSwitch(t *testing.T) {
	s := &Source{disabled: []registration{{resource: SecretsResource,
		schema: SecretSchema{}}}}
	assert.True(t, s.isDisabled(KindHelmRelease),
		"watch.secrets: false leaves Helm releases unverifiable")
	assert.False(t, (&Source{}).isDisabled(KindHelmRelease))
}
