package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/license"
)

var leaseStoreNow = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func TestBoltLeaseStoreKeepsLeaseAndGuardTogether(t *testing.T) {
	disk := diskState{store: openTestStore(t)}
	store := boltLeaseStore{disk: disk}
	guard := license.ClockGuard{MaxSeen: leaseStoreNow}
	require.NoError(t, store.SaveLease("KWL1-token", guard))
	require.NoError(t, disk.put(stateLicenseClock, license.ClockGuard{}))

	lease, got, err := store.LoadLease()
	require.NoError(t, err)
	require.Equal(t, "KWL1-token", lease)
	require.True(t, got.MaxSeen.Equal(leaseStoreNow),
		"an emptied old clock entry does not lower the guard")
}

func TestBoltLeaseStoreReadsTheOlderLayout(t *testing.T) {
	disk := diskState{store: openTestStore(t)}
	require.NoError(t, disk.put(stateLicenseLease, "KWL1-old"))
	require.NoError(t, disk.put(stateLicenseClock,
		license.ClockGuard{MaxSeen: leaseStoreNow}))

	lease, got, err := boltLeaseStore{disk: disk}.LoadLease()
	require.NoError(t, err)
	require.Equal(t, "KWL1-old", lease)
	require.True(t, got.MaxSeen.Equal(leaseStoreNow))
}

// With persistence.emptyDir the state file is new after a restart; the
// Secret copy brings the guard (and a verified lease) back.
func TestMirroredLeaseStoreSurvivesALostDataVolume(t *testing.T) {
	client := fake.NewSimpleClientset(emptyInstanceSecret("kwatch"))
	secret := kubeInstanceStore{client: client, namespace: "kwatch"}
	first := &mirroredLeaseStore{
		local:  boltLeaseStore{disk: diskState{store: openTestStore(t)}},
		secret: secret,
	}
	guard := license.ClockGuard{MaxSeen: leaseStoreNow}
	require.NoError(t, first.SaveLease("KWL1-token", guard))

	raw, err := client.CoreV1().Secrets("kwatch").Get(context.Background(),
		licenseInstanceSecret, metav1.GetOptions{})
	require.NoError(t, err)
	require.Contains(t, string(raw.Data[licenseLeaseCacheField]),
		"KWL1-token")

	fresh := &mirroredLeaseStore{
		local:  boltLeaseStore{disk: diskState{store: openTestStore(t)}},
		secret: secret,
	}
	_, got, err := fresh.LoadLease()
	require.NoError(t, err)
	require.True(t, got.MaxSeen.Equal(leaseStoreNow),
		"the guard comes back from the Secret")
}

func TestMirroredLeaseStoreWritesTheSecretOnlyOnChange(t *testing.T) {
	client := fake.NewSimpleClientset(emptyInstanceSecret("kwatch"))
	store := &mirroredLeaseStore{
		local:  boltLeaseStore{disk: diskState{store: openTestStore(t)}},
		secret: kubeInstanceStore{client: client, namespace: "kwatch"},
	}
	guard := license.ClockGuard{MaxSeen: leaseStoreNow}
	require.NoError(t, store.SaveLease("KWL1-a", guard))
	writes := func() int {
		n := 0
		for _, a := range client.Actions() {
			if a.GetVerb() == "update" {
				n++
			}
		}
		return n
	}
	guard.MaxSeen = guard.MaxSeen.Add(time.Minute)
	require.NoError(t, store.SaveLease("KWL1-a", guard))
	require.Equal(t, 1, writes(), "a small guard move is not mirrored")
	guard.MaxSeen = guard.MaxSeen.Add(leaseMirrorEvery)
	require.NoError(t, store.SaveLease("KWL1-a", guard))
	require.NoError(t, store.SaveLease("KWL1-b", guard))
	require.Equal(t, 3, writes())
}
