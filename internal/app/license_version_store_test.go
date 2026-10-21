package app

import (
	"testing"
	"time"

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

func TestMirroredLeaseStoreRenewAttemptSurvivesALostDataVolume(t *testing.T) {
	client := fake.NewSimpleClientset(emptyInstanceSecret("kwatch"))
	secret := kubeInstanceStore{client: client, namespace: "kwatch"}
	first := &mirroredLeaseStore{
		local:  boltLeaseStore{disk: diskState{store: openTestStore(t)}},
		secret: secret,
	}
	want := license.RenewAttempt{Version: "v1.3.0",
		At: leaseStoreNow.Truncate(time.Second), Count: 2}
	require.NoError(t, first.SaveRenewAttempt(want))
	got, err := first.local.LoadRenewAttempt()
	require.NoError(t, err)
	require.Equal(t, want, got)

	fresh := &mirroredLeaseStore{
		local:  boltLeaseStore{disk: diskState{store: openTestStore(t)}},
		secret: secret,
	}
	got, err = fresh.LoadRenewAttempt()
	require.NoError(t, err)
	require.True(t, want.At.Equal(got.At))
	require.Equal(t, want.Count, got.Count)

	require.NoError(t, fresh.SaveRenewAttempt(license.RenewAttempt{}))
	got, err = fresh.LoadRenewAttempt()
	require.NoError(t, err)
	require.Empty(t, got.Version, "success clears both copies")
}
