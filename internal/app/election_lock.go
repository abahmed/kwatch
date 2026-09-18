package app

import (
	"context"
	"fmt"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/k8s"
)

type renewalTrackingLock struct {
	delegate  resourcelock.Interface
	onRenewal func(time.Time)
}

func (l *renewalTrackingLock) Get(
	ctx context.Context,
) (*resourcelock.LeaderElectionRecord, []byte, error) {
	return l.delegate.Get(ctx)
}

func (l *renewalTrackingLock) Create(
	ctx context.Context,
	record resourcelock.LeaderElectionRecord,
) error {
	err := l.delegate.Create(ctx, record)
	if err == nil {
		l.recordRenewal(record)
	}
	return err
}

func (l *renewalTrackingLock) Update(
	ctx context.Context,
	record resourcelock.LeaderElectionRecord,
) error {
	err := l.delegate.Update(ctx, record)
	if err == nil {
		l.recordRenewal(record)
	}
	return err
}

func (l *renewalTrackingLock) recordRenewal(
	record resourcelock.LeaderElectionRecord,
) {
	if l.onRenewal != nil && !record.RenewTime.IsZero() {
		l.onRenewal(record.RenewTime.Time)
	}
}

func (l *renewalTrackingLock) RecordEvent(event string) {
	l.delegate.RecordEvent(event)
}

func (l *renewalTrackingLock) Identity() string {
	return l.delegate.Identity()
}

func (l *renewalTrackingLock) Describe() string {
	return l.delegate.Describe()
}

func newLeaseLock(
	client kubernetes.Interface,
	identity string,
) resourcelock.Interface {
	return &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      electionLeaseName(),
			Namespace: k8s.GetNamespace(),
		},
		Client: client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: identity,
		},
	}
}

func electionLeaseName() string {
	if name := os.Getenv("KWATCH_LEADER_ELECTION_NAME"); name != "" {
		return name
	}
	if installation := os.Getenv("KWATCH_INSTALLATION_ID"); installation != "" {
		return installation + "-leader"
	}
	return leaderLeaseName
}

func podIdentity() (string, error) {
	if identity := os.Getenv("POD_NAME"); identity != "" {
		return identity, nil
	}
	identity, err := os.Hostname()
	if err != nil || identity == "" {
		return "", fmt.Errorf("leader election requires Pod identity: %w", err)
	}
	klog.InfoS("using host identity for leader election", "identity", identity)
	return identity, nil
}
