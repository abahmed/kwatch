package app

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/license"
)

var (
	_ license.VersionStore = boltLeaseStore{}
	_ license.VersionStore = (*mirroredLeaseStore)(nil)
)

func TestBoltLeaseStoreKeepsTheReportedVersion(t *testing.T) {
	store := boltLeaseStore{disk: diskState{store: openTestStore(t)}}
	v, err := store.LoadReportedVersion()
	require.NoError(t, err)
	require.Empty(t, v)
	require.NoError(t, store.SaveReportedVersion("v1.2.3"))
	v, err = store.LoadReportedVersion()
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", v)
}

func TestMirroredLeaseStoreVersionSurvivesALostDataVolume(t *testing.T) {
	client := fake.NewSimpleClientset(emptyInstanceSecret("kwatch"))
	secret := kubeInstanceStore{client: client, namespace: "kwatch"}
	first := &mirroredLeaseStore{
		local:  boltLeaseStore{disk: diskState{store: openTestStore(t)}},
		secret: secret,
	}
	require.NoError(t, first.SaveReportedVersion("v1.2.3"))
	require.NoError(t, first.SaveReportedVersion("v1.2.3"))
	updates := 0
	for _, a := range client.Actions() {
		if a.GetVerb() == "update" {
			updates++
		}
	}
	require.Equal(t, 1, updates, "an unchanged version is not rewritten")

	fresh := &mirroredLeaseStore{
		local:  boltLeaseStore{disk: diskState{store: openTestStore(t)}},
		secret: secret,
	}
	v, err := fresh.LoadReportedVersion()
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", v, "the version comes back from the Secret")
}
