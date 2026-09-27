package app

import (
	"context"
	"testing"
	"time"

	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

// fakeResourceLock is a minimal resourcelock.Interface recording Update
// calls, enough to exercise releaseLease without a real Lease object.
type fakeResourceLock struct {
	record       *resourcelock.LeaderElectionRecord
	updateCalled bool
	updated      resourcelock.LeaderElectionRecord
}

func (l *fakeResourceLock) Get(
	context.Context,
) (*resourcelock.LeaderElectionRecord, []byte, error) {
	return l.record, nil, nil
}

func (l *fakeResourceLock) Create(
	context.Context, resourcelock.LeaderElectionRecord,
) error {
	return nil
}

func (l *fakeResourceLock) Update(
	_ context.Context, record resourcelock.LeaderElectionRecord,
) error {
	l.updateCalled = true
	l.updated = record
	return nil
}

func (l *fakeResourceLock) RecordEvent(string) {}

func (l *fakeResourceLock) Identity() string { return "fake" }

func (l *fakeResourceLock) Describe() string { return "fake" }

func TestReleaseLeaseUpdatesRecordWhenHeldByIdentity(t *testing.T) {
	lock := &fakeResourceLock{
		record: &resourcelock.LeaderElectionRecord{
			HolderIdentity: "me",
		},
	}

	releaseLease(context.Background(), lock, "me", time.Now)

	if !lock.updateCalled {
		t.Fatalf("expected Update to be called when this replica holds it")
	}
	if lock.updated.HolderIdentity != "" {
		t.Fatalf("expected HolderIdentity cleared, got %q",
			lock.updated.HolderIdentity)
	}
	if lock.updated.LeaseDurationSeconds != 1 {
		t.Fatalf("expected LeaseDurationSeconds 1, got %d",
			lock.updated.LeaseDurationSeconds)
	}
}

func TestReleaseLeaseSkipsUpdateWhenHeldByOther(t *testing.T) {
	lock := &fakeResourceLock{
		record: &resourcelock.LeaderElectionRecord{
			HolderIdentity: "other",
		},
	}

	releaseLease(context.Background(), lock, "me", time.Now)

	if lock.updateCalled {
		t.Fatalf("did not expect Update when another identity holds it")
	}
}

func TestInstallationStatePrefixDefault(t *testing.T) {
	t.Setenv("KWATCH_LEADER_ELECTION_NAME", "")
	t.Setenv("KWATCH_INSTALLATION_ID", "")

	if got := installationStatePrefix(); got != "kwatch" {
		t.Fatalf("expected default prefix %q, got %q", "kwatch", got)
	}
}

func TestInstallationStatePrefixFromLeaseName(t *testing.T) {
	t.Setenv("KWATCH_LEADER_ELECTION_NAME", "team-x-leader")
	t.Setenv("KWATCH_INSTALLATION_ID", "")

	if got := installationStatePrefix(); got != "team-x" {
		t.Fatalf("expected prefix %q, got %q", "team-x", got)
	}
}
