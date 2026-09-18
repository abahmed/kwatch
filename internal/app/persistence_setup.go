package app

import (
	"context"
	"fmt"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/health"
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
	restoreErr    error
	feedbackDone  chan struct{}
	saveFeedback  func()
	activate      func(context.Context, func(map[string]map[string]int64)) error
	start         func(
		context.Context, *componentSupervisor, func() bool,
	)
}

func configurePersistence(
	ctx context.Context,
	persistenceManager *persistence.Manager,
	runtime config.RuntimeConfig,
	now func() time.Time,
	healthServer *health.HealthServer,
) persistenceSetup {
	persistenceManager.BeginMigrationReport()
	tracker, trackerErr := loadChangeTracker(ctx, persistenceManager, now)
	recordOptionalRestore(
		persistenceManager, "change-history", trackerErr,
	)
	changeDone := make(chan struct{})

	baselineCh := make(chan map[string]map[string]int64, 64)
	baseline, baselineErr := persistenceManager.GetBaselineWithError(ctx)
	recordRequiredRestore(
		persistenceManager, "baseline", baselineErr,
	)
	baselineDone := make(chan struct{})

	incidentDone := make(chan struct{})
	incidentCh := make(chan stateSnapshot, 1)
	var incidentSaverForRun incidentSaver = persistenceManager

	feedbackStore, feedbackErr := loadFeedbackStore(
		ctx, persistenceManager, now,
	)
	recordOptionalRestore(
		persistenceManager, "feedback", feedbackErr,
	)
	var pvcErr error
	if runtime.Monitors().PVC().Enabled {
		_, pvcErr = persistenceManager.GetPvcUsageWithError(ctx)
		recordRequiredRestore(persistenceManager, "pvc-state", pvcErr)
	} else {
		recordNotRequiredRestore(persistenceManager, "pvc-state")
	}
	if runtime.Monitors().KubeletTelemetry().PersistState {
		_, telemetryErr := persistenceManager.LoadTelemetryState(ctx)
		recordOptionalRestore(
			persistenceManager, "telemetry", telemetryErr,
		)
	} else {
		recordNotRequiredRestore(persistenceManager, "telemetry")
	}
	feedbackDone := make(chan struct{})
	feedbackCh := make(chan []insight.RCARecord, 1)
	start := func(
		ctx context.Context,
		supervisor *componentSupervisor,
		canWrite func() bool,
	) {
		supervisor.startOwned(ctx, componentSpec{
			name:     "change-history-saver",
			required: true,
			onHealthy: func() {
				if healthServer != nil {
					healthServer.SetComponentStatus(
						"change-history-saver", "running", "", true,
					)
				}
			},
			run: func(ctx context.Context) error {
				defer close(changeDone)
				return startChangeHistorySaver(
					ctx, persistenceManager, tracker,
					persistenceStatus(
						healthServer, "change-history-saver", true,
					),
					canWrite,
				)
			},
		})
		supervisor.startOwned(ctx, componentSpec{
			name:     "baseline-saver",
			required: true,
			onHealthy: func() {
				if healthServer != nil {
					healthServer.SetComponentStatus(
						"baseline-saver", "running", "", true,
					)
				}
			},
			run: func(ctx context.Context) error {
				defer close(baselineDone)
				return startBaselineSaverWithStatus(
					ctx, persistenceManager, baselineCh, 0,
					persistenceStatus(healthServer, "baseline-saver", true),
					canWrite,
				)
			},
		})
		supervisor.startOwned(ctx, componentSpec{
			name:     "incident-saver",
			required: true,
			onHealthy: func() {
				if healthServer != nil {
					healthServer.SetComponentStatus(
						"incident-saver", "running", "", true,
					)
				}
			},
			run: func(ctx context.Context) error {
				defer close(incidentDone)
				return startIncidentSaver(
					ctx, persistenceManager, incidentCh,
					persistenceStatus(healthServer, "incident-saver", true),
					canWrite,
				)
			},
		})
		supervisor.startOwned(ctx, componentSpec{
			name: "feedback-saver",
			onHealthy: func() {
				if healthServer != nil {
					healthServer.SetComponentStatus(
						"feedback-saver", "running", "", true,
					)
				}
			},
			run: func(ctx context.Context) error {
				startFeedbackSaver(
					ctx, persistenceManager, feedbackCh, feedbackDone,
					persistenceStatus(healthServer, "feedback-saver", false),
					canWrite,
				)
				return nil
			},
		})
	}

	setup := persistenceSetup{
		tracker: tracker, baseline: baseline, baselineCh: baselineCh,
		baselineDone: baselineDone, changeDone: changeDone,
		incidentCh: incidentCh, incidentDone: incidentDone,
		incidentSaver: incidentSaverForRun, feedbackStore: feedbackStore,
		restoreErr:   pvcErr,
		feedbackDone: feedbackDone,
		saveFeedback: feedbackSnapshotSaver(feedbackCh, feedbackStore),
		start:        start,
	}
	setup.activate = func(
		ctx context.Context,
		applyBaseline func(map[string]map[string]int64),
	) error {
		if baselineErr != nil || setup.restoreErr != nil {
			restoreErr := baselineErr
			if restoreErr == nil {
				restoreErr = setup.restoreErr
			}
			if healthServer != nil {
				healthServer.SetComponentError("persistence", restoreErr)
			}
			return fmt.Errorf("restore required state: %w", restoreErr)
		}
		result, err := persistenceManager.
			MigrateLegacyBaselineWithResult(ctx)
		if err != nil {
			klog.ErrorS(err, "failed to migrate legacy baseline")
			if healthServer != nil {
				healthServer.SetComponentError("persistence", err)
			}
			return err
		}
		klog.InfoS(
			"legacy baseline migration checked",
			"status", result.Status,
			"detail", result.Detail,
		)
		if healthServer != nil {
			available := result.Status != persistence.MigrationFailed &&
				result.Status != persistence.MigrationUnsupported
			state := "running"
			reason := ""
			if !available {
				state = "degraded"
				reason = "persistence_migration_failed"
			}
			healthServer.SetComponentStatus(
				"persistence", state, reason, available,
			)
		}
		if result.Status == persistence.MigrationFailed ||
			result.Status == persistence.MigrationUnsupported {
			return fmt.Errorf(
				"persistence migration %s", result.Status,
			)
		}
		if applyBaseline != nil {
			applyBaseline(baseline)
		}
		return nil
	}
	return setup
}

func persistenceStatus(
	healthServer *health.HealthServer,
	name string,
	required bool,
) func(error) {
	return func(err error) {
		if healthServer == nil {
			return
		}
		healthServer.SetComponentError(name, err)
		if required && err != nil {
			healthServer.SetReady(false)
		}
	}
}

func loadChangeTracker(
	ctx context.Context,
	persistenceManager persistence.ChangeHistoryStore,
	now func() time.Time,
) (*kwcontext.ChangeTracker, error) {
	tracker := kwcontext.NewChangeTrackerWithClock(0, clock.Func(now))
	changes, err := persistenceManager.LoadChangeHistory(ctx)
	if err == nil {
		tracker.Restore(changes)
	}
	return tracker, err
}

func loadFeedbackStore(
	ctx context.Context,
	persistenceManager persistence.FeedbackStore,
	now func() time.Time,
) (*insight.FeedbackStore, error) {
	store := insight.NewFeedbackStoreWithClock(clock.Func(now))
	records, err := persistenceManager.LoadRCAFeedback(ctx)
	if err == nil {
		store.Restore(records)
	}
	return store, err
}

func recordOptionalRestore(
	manager *persistence.Manager,
	store string,
	err error,
) {
	status := persistence.MigrationCompleted
	detail := "restored optional persisted state"
	if err != nil {
		status = persistence.MigrationFailed
		detail = "optional persisted state unavailable"
	}
	manager.RecordMigrationResult(persistence.MigrationResult{
		Store:                 store,
		SourceFormat:          "kwatch-" + store,
		DestinationFormat:     "runtime/" + store,
		Status:                status,
		Recoverable:           true,
		MonitoringMayContinue: true,
		Detail:                detail,
	}, err)
}

func recordRequiredRestore(
	manager *persistence.Manager,
	store string,
	err error,
) {
	status := persistence.MigrationCompleted
	detail := "restored persisted state"
	if err != nil {
		status = persistence.MigrationFailed
		detail = "required persisted state unavailable"
	}
	manager.RecordMigrationResult(persistence.MigrationResult{
		Store:                 store,
		SourceFormat:          "kwatch-" + store,
		DestinationFormat:     "runtime/" + store,
		Status:                status,
		Recoverable:           err == nil,
		MonitoringMayContinue: err == nil,
		Detail:                detail,
	}, err)
}

func recordNotRequiredRestore(
	manager *persistence.Manager,
	store string,
) {
	manager.RecordMigrationResult(persistence.MigrationResult{
		Store:                 store,
		SourceFormat:          "not-required",
		DestinationFormat:     "runtime/" + store,
		Status:                persistence.MigrationNotRequired,
		Recoverable:           true,
		MonitoringMayContinue: true,
		Detail:                "state is not required by active configuration",
	}, nil)
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
	report func(error),
	canWrite func() bool,
) error {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if !writesAllowed(canWrite) {
				return nil
			}
			if err := persistenceManager.SaveChangeHistory(
				ctx,
				tracker.Snapshot(),
			); err != nil {
				klog.ErrorS(err, "failed to persist recent change history")
				if report != nil {
					report(err)
				}
				return err
			} else if report != nil {
				report(nil)
			}
		}
	}
}
