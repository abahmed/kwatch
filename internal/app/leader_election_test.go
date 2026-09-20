package app

import (
	"context"
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"

	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/health"
)

type fakeElection struct {
	callbacks leaderelection.LeaderCallbacks
	lose      <-chan struct{}
}

func (e *fakeElection) Run(ctx context.Context) {
	leaderCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go e.callbacks.OnStartedLeading(leaderCtx)
	select {
	case <-ctx.Done():
	case <-e.lose:
		cancel()
	}
	e.callbacks.OnStoppedLeading()
}

func testServerDeps() *serverDeps {
	deps := &serverDeps{
		clients: client.ClientSet{
			Kubernetes: fake.NewSimpleClientset(),
			Clock:      clock.RealClock{},
		},
		healthServer: health.NewHealthServerWithClock(
			config.HealthCheck{}, clock.RealClock{},
		),
	}
	deps.persistenceGate = newPersistenceGate()
	return deps
}

func TestLeaderElectionStartsActiveSessionAfterAcquisition(t *testing.T) {
	t.Setenv("POD_NAME", "kwatch-a")
	t.Setenv("POD_NAMESPACE", "kwatch")
	t.Setenv("KWATCH_LEADER_ELECTION_NAME", "kwatch-test-leader")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	deps := testServerDeps()
	activeStarted := make(chan struct{})
	var gotConfig leaderelection.LeaderElectionConfig
	factory := func(
		config leaderelection.LeaderElectionConfig,
	) (electionRunner, error) {
		gotConfig = config
		return &fakeElection{
			callbacks: config.Callbacks,
			lose:      make(chan struct{}),
		}, nil
	}
	activeRunner := func(ctx context.Context, _ *serverDeps) error {
		close(activeStarted)
		<-ctx.Done()
		return nil
	}
	result := make(chan error, 1)
	go func() {
		result <- runLeaderElectionWithRunner(
			ctx, deps, factory, activeRunner,
		)
	}()

	select {
	case <-activeStarted:
	case <-time.After(time.Second):
		t.Fatal("active session did not start")
	}
	assertLeaderElectionConfig(t, gotConfig, deps)

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("leader election returned %v on cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("leader election did not stop")
	}
	if deps.healthServer.Ready() {
		t.Fatal("leader remained ready after cancellation")
	}
}

func assertLeaderElectionConfig(
	t *testing.T,
	electionConfig leaderelection.LeaderElectionConfig,
	deps *serverDeps,
) {
	t.Helper()
	status := deps.healthServer.LeadershipStatus()
	if status == nil || status.Role != "leader" {
		t.Fatalf("leadership status = %+v, want leader", status)
	}
	if status.Identity != "kwatch-a" || status.Epoch != 1 {
		t.Fatalf("unexpected leader status: %+v", status)
	}
	if electionConfig.LeaseDuration != leaderLeaseDuration ||
		electionConfig.RenewDeadline != leaderRenewDeadline ||
		electionConfig.RetryPeriod != leaderRetryPeriod {
		t.Fatalf("unexpected election timings: %+v", electionConfig)
	}
	if !electionConfig.ReleaseOnCancel {
		t.Fatal("voluntary shutdown should release the Lease")
	}
	tracking, ok := electionConfig.Lock.(*renewalTrackingLock)
	if !ok {
		t.Fatalf("election lock type = %T", electionConfig.Lock)
	}
	lock, ok := tracking.delegate.(*resourcelock.LeaseLock)
	if !ok {
		t.Fatalf("delegate lock type = %T", tracking.delegate)
	}
	if lock.LeaseMeta.Name != "kwatch-test-leader" ||
		lock.LeaseMeta.Namespace != "kwatch" ||
		lock.Identity() != "kwatch-a" {
		t.Fatalf("unexpected election lock: %+v", lock.LeaseMeta)
	}
	renewal := time.Now().UTC()
	if err := tracking.Create(
		context.Background(),
		resourcelock.LeaderElectionRecord{
			RenewTime: metav1.NewTime(renewal),
		},
	); err != nil {
		t.Fatalf("create lease: %v", err)
	}
	status = deps.healthServer.LeadershipStatus()
	if status == nil || !status.LastRenewal.Equal(renewal) {
		t.Fatalf("last renewal = %v, want %v", status, renewal)
	}
}

func TestLeaderElectionRecordsTakeoverAfterObservedLeader(t *testing.T) {
	t.Setenv("POD_NAME", "kwatch-b")
	t.Setenv("POD_NAMESPACE", "kwatch")
	t.Setenv("KWATCH_INSTALLATION_ID", "kwatch")
	deps := testServerDeps()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	factory := func(
		config leaderelection.LeaderElectionConfig,
	) (electionRunner, error) {
		return &observingElection{
			callbacks: config.Callbacks,
		}, nil
	}
	active := func(ctx context.Context, _ *serverDeps) error {
		close(started)
		<-ctx.Done()
		return nil
	}
	result := make(chan error, 1)
	go func() {
		result <- runLeaderElectionWithRunner(ctx, deps, factory, active)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("leader was not started")
	}
	status := deps.healthServer.LeadershipStatus()
	if status == nil || status.TakeoverCount != 1 {
		t.Fatalf("leadership status = %+v, want takeover", status)
	}
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("election returned %v on cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("election did not stop")
	}
}

type observingElection struct {
	callbacks leaderelection.LeaderCallbacks
}

func (e *observingElection) Run(ctx context.Context) {
	leaderCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	e.callbacks.OnNewLeader("kwatch-a")
	go func() {
		e.callbacks.OnStartedLeading(leaderCtx)
	}()
	<-ctx.Done()
	e.callbacks.OnStoppedLeading()
}

func TestLeaderElectionReturnsLeadershipLoss(t *testing.T) {
	t.Setenv("POD_NAME", "kwatch-a")
	t.Setenv("POD_NAMESPACE", "kwatch")
	t.Setenv("KWATCH_INSTALLATION_ID", "kwatch")
	deps := testServerDeps()
	ctx := context.Background()
	lose := make(chan struct{})
	activeStarted := make(chan struct{})
	factory := func(
		config leaderelection.LeaderElectionConfig,
	) (electionRunner, error) {
		return &fakeElection{callbacks: config.Callbacks, lose: lose}, nil
	}
	activeRunner := func(ctx context.Context, _ *serverDeps) error {
		close(activeStarted)
		<-ctx.Done()
		return nil
	}
	result := make(chan error, 1)
	go func() {
		result <- runLeaderElectionWithRunner(
			ctx, deps, factory, activeRunner,
		)
	}()
	select {
	case <-activeStarted:
	case <-time.After(time.Second):
		t.Fatal("active session did not start")
	}
	close(lose)
	select {
	case err := <-result:
		if !errors.Is(err, errLeadershipLost) {
			t.Fatalf("error = %v, want leadership loss", err)
		}
	case <-time.After(time.Second):
		t.Fatal("leadership loss was not reported")
	}
	if got := deps.healthServer.LeadershipStatus(); got == nil ||
		got.Role != "stopped" || got.LossReason != "leadership_lost" {
		t.Fatalf("unexpected stopped status: %+v", got)
	}
	if deps.persistenceGate == nil || deps.persistenceGate.enabled() {
		t.Fatal("leadership loss left persistence writes enabled")
	}
}

func TestLeaderElectionRequiresKubernetesClient(t *testing.T) {
	deps := testServerDeps()
	deps.clients.Kubernetes = nil
	err := runLeaderElectionWithRunner(
		context.Background(), deps, func(
			leaderelection.LeaderElectionConfig,
		) (electionRunner, error) {
			return nil, nil
		}, func(context.Context, *serverDeps) error { return nil },
	)
	if err == nil || err.Error() != "leader election requires Kubernetes client" {
		t.Fatalf("error = %v", err)
	}
}

func TestNewLeaseLockUsesConfiguredIdentity(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "monitoring")
	t.Setenv("KWATCH_LEADER_ELECTION_NAME", "kwatch-election")
	var kubernetesClient kubernetes.Interface = fake.NewSimpleClientset()
	lock := newLeaseLock(kubernetesClient, "pod-1").(*resourcelock.LeaseLock)
	if lock.LeaseMeta.Name != "kwatch-election" ||
		lock.LeaseMeta.Namespace != "monitoring" ||
		lock.Identity() != "pod-1" {
		t.Fatalf("unexpected lock: %+v", lock.LeaseMeta)
	}
}
