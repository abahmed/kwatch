package app

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/persistence"
)

type persistenceSetup struct {
	tracker       *kwcontext.ChangeTracker
	baseline      map[string]map[string]int64
	baselineCh    chan map[string]map[string]int64
	baselineDone  chan struct{}
	changeDone    chan struct{}
	incidentCh    chan stateSnapshot
	incidentDone  chan struct{}
	incidentSaver incidentSaver
	feedbackStore *insight.FeedbackStore
	feedbackDone  chan struct{}
	saveFeedback  func()
	start         func(context.Context, *componentSupervisor)
}

func configurePersistence(
	ctx context.Context,
	persistenceManager *persistence.Manager,
	now func() time.Time,
) persistenceSetup {
	tracker := loadChangeTracker(ctx, persistenceManager, now)
	changeDone := make(chan struct{})

	baselineCh := make(chan map[string]map[string]int64, 64)
	// Migrate before loading: the first release using the dedicated ConfigMap
	// may still have its baseline in the legacy state object.
	result, err := persistenceManager.MigrateLegacyBaselineWithResult(ctx)
	if err != nil {
		klog.ErrorS(err, "failed to migrate legacy baseline")
	} else {
		klog.InfoS(
			"legacy baseline migration checked",
			"status", result.Status,
			"detail", result.Detail,
		)
	}
	baseline := persistenceManager.GetBaseline(ctx)
	baselineDone := make(chan struct{})

	incidentDone := make(chan struct{})
	incidentCh := make(chan stateSnapshot, 1)
	var incidentSaverForRun incidentSaver = persistenceManager

	feedbackStore := loadFeedbackStore(ctx, persistenceManager, now)
	feedbackDone := make(chan struct{})
	feedbackCh := make(chan []insight.RCARecord, 1)
	start := func(ctx context.Context, supervisor *componentSupervisor) {
		supervisor.startOwned(ctx, componentSpec{
			name: "change-history-saver",
			run: func(ctx context.Context) error {
				defer close(changeDone)
				startChangeHistorySaver(ctx, persistenceManager, tracker)
				return nil
			},
		})
		supervisor.startOwned(ctx, componentSpec{
			name: "baseline-saver",
			run: func(ctx context.Context) error {
				defer close(baselineDone)
				startBaselineSaver(ctx, persistenceManager, baselineCh, 0)
				return nil
			},
		})
		supervisor.startOwned(ctx, componentSpec{
			name: "incident-saver",
			run: func(ctx context.Context) error {
				defer close(incidentDone)
				startIncidentSaver(ctx, persistenceManager, incidentCh)
				return nil
			},
		})
		supervisor.startOwned(ctx, componentSpec{
			name: "feedback-saver",
			run: func(ctx context.Context) error {
				startFeedbackSaver(
					ctx, persistenceManager, feedbackCh, feedbackDone,
				)
				return nil
			},
		})
	}

	return persistenceSetup{
		tracker: tracker, baseline: baseline, baselineCh: baselineCh,
		baselineDone: baselineDone, changeDone: changeDone,
		incidentCh: incidentCh, incidentDone: incidentDone,
		incidentSaver: incidentSaverForRun, feedbackStore: feedbackStore,
		feedbackDone: feedbackDone,
		saveFeedback: feedbackSnapshotSaver(feedbackCh, feedbackStore),
		start:        start,
	}
}

func loadChangeTracker(
	ctx context.Context,
	persistenceManager persistence.ChangeHistoryStore,
	now func() time.Time,
) *kwcontext.ChangeTracker {
	tracker := kwcontext.NewChangeTrackerWithClock(0, clock.Func(now))
	if changes, err := persistenceManager.LoadChangeHistory(ctx); err == nil {
		tracker.Restore(changes)
	}
	return tracker
}

func loadFeedbackStore(
	ctx context.Context,
	persistenceManager persistence.FeedbackStore,
	now func() time.Time,
) *insight.FeedbackStore {
	store := insight.NewFeedbackStoreWithClock(clock.Func(now))
	if records, err := persistenceManager.LoadRCAFeedback(ctx); err == nil {
		store.Restore(records)
	}
	return store
}

func feedbackSnapshotSaver(
	ch chan []insight.RCARecord,
	store *insight.FeedbackStore,
) func() {
	return func() { trySendFeedbackSnapshot(ch, store.Snapshot()) }
}

func startChangeHistorySaver(
	ctx context.Context,
	persistenceManager persistence.ChangeHistoryStore,
	tracker *kwcontext.ChangeTracker,
) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := persistenceManager.SaveChangeHistory(
				ctx,
				tracker.Snapshot(),
			); err != nil {
				klog.ErrorS(err, "failed to persist recent change history")
			}
		}
	}
}
