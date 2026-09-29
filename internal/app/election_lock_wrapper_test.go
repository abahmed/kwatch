package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

func TestRenewalTrackingLockReportsSuccessfulWrites(t *testing.T) {
	delegate := &fakeResourceLock{}
	var renewals []time.Time
	lock := &renewalTrackingLock{
		delegate:  delegate,
		onRenewal: func(at time.Time) { renewals = append(renewals, at) },
	}
	renewed := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	record := resourcelock.LeaderElectionRecord{
		RenewTime: metaTime(renewed),
	}

	require.NoError(t, lock.Create(context.Background(), record))
	require.NoError(t, lock.Update(context.Background(), record))
	require.NoError(t, lock.Update(context.Background(),
		resourcelock.LeaderElectionRecord{}))

	require.Len(t, renewals, 2, "zero renew times are not renewals")
	require.True(t, renewals[0].Equal(renewed))
}

func TestRenewalTrackingLockDelegatesReads(t *testing.T) {
	record := &resourcelock.LeaderElectionRecord{HolderIdentity: "me"}
	lock := &renewalTrackingLock{
		delegate: &fakeResourceLock{record: record},
	}

	got, _, err := lock.Get(context.Background())
	lock.RecordEvent("x")

	require.NoError(t, err)
	require.Equal(t, "me", got.HolderIdentity)
	require.Equal(t, "fake", lock.Identity())
	require.Equal(t, "fake", lock.Describe())
}

func TestPodIdentityFallsBackToHostname(t *testing.T) {
	t.Setenv("POD_NAME", "")

	identity, err := podIdentity()

	require.NoError(t, err)
	require.NotEmpty(t, identity)
}

func metaTime(at time.Time) metav1.Time { return metav1.NewTime(at) }
