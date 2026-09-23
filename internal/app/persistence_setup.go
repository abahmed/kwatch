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
	feedbackDone  chan struct{}
	saveFeedback  func()
	activate      func(context.Context, func(map[string]map[string]int64)) error
	start         func(
		context.Context, *componentSupervisor, func() bool,
	)
}

func configurePersistence(
	persistenceManager *persistence.Manager,
	runtime config.RuntimeConfig,
	now func() time.Time,
	healthServer *health.HealthServer,
	readiness *readinessCoordinator,
) persistenceSetup {
	tracker := kwcontext.NewChangeTrackerWithClock(0, clock.Func(now))
	baseline := make(map[string]map[string]int64)
	feedbackStore := insight.NewFeedbackStoreWithClock(clock.Func(now))

	baselineCh := make(chan map[string]map[string]int64, 64)
	baselineDone := make(chan struct{})

	changeDone := make(chan struct{})
	incidentDone := make(chan struct{})
	incidentCh := make(chan stateSnapshot, 1)
	var incidentSaverForRun incidentSaver = persistenceManager

	feedbackDone := make(chan struct{})
	feedbackCh := make(chan []insight.RCARecord, 1)
	start := func(
		ctx context.Context,
		supervisor *componentSupervisor,
		canWrite func() bool,
	) {
		startPersistenceSavers(
			ctx, supervisor, persistenceManager, tracker, baselineCh,
			incidentCh, feedbackCh, changeDone, baselineDone, incidentDone,
			feedbackDone, canWrite, now, healthServer, readiness,
		)
	}

	setup := persistenceSetup{
		tracker: tracker, baseline: baseline, baselineCh: baselineCh,
		baselineDone: baselineDone, changeDone: changeDone,
		incidentCh: incidentCh, incidentDone: incidentDone,
		incidentSaver: incidentSaverForRun, feedbackStore: feedbackStore,
		feedbackDone: feedbackDone,
		saveFeedback: feedbackSnapshotSaver(feedbackCh, feedbackStore),
		start:        start,
	}
	setup.activate = func(
		ctx context.Context,
		applyBaseline func(map[string]map[string]int64),
	) error {
		persistenceManager.BeginMigrationReport()
		restored := restorePersistenceState(
			ctx, persistenceManager, runtime, tracker, feedbackStore,
		)
		if restored.requiredErr != nil {
			restoreErr := restored.requiredErr
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
		// A legacy baseline is written by the migration after the initial
		// restore read. Reload it before activation so the incident engine
		// cannot start with a stale empty baseline.
		if result.Status == persistence.MigrationCompleted {
			baseline, reloadErr := persistenceManager.GetBaselineWithError(ctx)
			managerResult := persistence.MigrationResult{
				Store:                 "baseline",
				Operation:             persistence.OperationRecover,
				SourceFormat:          "kwatch-baseline/baseline",
				DestinationFormat:     "runtime/baseline",
				Status:                persistence.MigrationCompleted,
				Recoverable:           reloadErr == nil,
				MonitoringMayContinue: reloadErr == nil,
				Detail:                "reloaded migrated baseline",
			}
			if reloadErr != nil {
				managerResult.Status = persistence.MigrationFailed
				managerResult.Detail = "migrated baseline could not be reloaded"
			}
			persistenceManager.RecordMigrationResult(managerResult, reloadErr)
			if reloadErr != nil {
				return fmt.Errorf("reload migrated baseline: %w", reloadErr)
			}
			restored.baseline = baseline
		}
		if applyBaseline != nil {
			applyBaseline(restored.baseline)
		}
		return nil
	}
	return setup
}

func persistenceStatus(
	healthServer *health.HealthServer,
	name string,
	required bool,
	readiness *readinessCoordinator,
) func(error) {
	return func(err error) {
		if healthServer == nil {
			return
		}
		healthServer.SetComponentError(name, err)
		if required && err != nil {
			healthServer.SetReady(false)
			if readiness != nil {
				readiness.writerFailed(name)
			}
		}
	}
}

type restoredPersistenceState struct {
	baseline    map[string]map[string]int64
	requiredErr error
}

func restorePersistenceState(
	ctx context.Context,
	manager *persistence.Manager,
	runtime config.RuntimeConfig,
	tracker *kwcontext.ChangeTracker,
	feedbackStore *insight.FeedbackStore,
) restoredPersistenceState {
	trackerErr := restoreChangeTracker(ctx, manager, tracker)
	recordOptionalRestore(manager, "change-history", trackerErr)
	baseline, baselineErr := manager.GetBaselineWithError(ctx)
	recordRequiredRestore(manager, "baseline", baselineErr)
	feedbackErr := restoreFeedbackStore(ctx, manager, feedbackStore)
	recordOptionalRestore(manager, "feedback", feedbackErr)

	requiredErr := baselineErr
	if runtime.Monitors().PVC().Enabled {
		_, pvcErr := manager.GetPvcUsageWithError(ctx)
		recordRequiredRestore(manager, "pvc-state", pvcErr)
		if requiredErr == nil {
			requiredErr = pvcErr
		}
	} else {
		recordNotRequiredRestore(manager, "pvc-state")
	}
	if runtime.Monitors().KubeletTelemetry().PersistState {
		_, telemetryErr := manager.LoadTelemetryState(ctx)
		recordOptionalRestore(manager, "telemetry", telemetryErr)
	} else {
		recordNotRequiredRestore(manager, "telemetry")
	}
	return restoredPersistenceState{
		requiredErr: requiredErr,
		baseline:    baseline,
	}
}

func startPersistenceSavers(
	ctx context.Context,
	supervisor *componentSupervisor,
	manager *persistence.Manager,
	tracker *kwcontext.ChangeTracker,
	baselineCh chan map[string]map[string]int64,
	incidentCh chan stateSnapshot,
	feedbackCh chan []insight.RCARecord,
	changeDone chan struct{},
	baselineDone chan struct{},
	incidentDone chan struct{},
	feedbackDone chan struct{},
	canWrite func() bool,
	now func() time.Time,
	healthServer *health.HealthServer,
	readiness *readinessCoordinator,
) {
	if readiness != nil {
		readiness.registerRequiredWriter("change-history-saver")
		readiness.registerRequiredWriter("baseline-saver")
		readiness.registerRequiredWriter("incident-saver")
	}
	startRequiredSaver(
		ctx, supervisor, "change-history-saver", changeDone,
		now, func() {
			if healthServer != nil {
				healthServer.SetComponentStatus(
					"change-history-saver", "running", "", true,
				)
			}
		}, func(ctx context.Context, progress func()) error {
			return startChangeHistorySaver(
				ctx, manager, tracker,
				persistenceStatus(
					healthServer, "change-history-saver", true, readiness,
				),
				canWrite, progress,
			)
		},
		readiness,
	)
	startRequiredSaver(
		ctx, supervisor, "baseline-saver", baselineDone,
		now, func() {
			if healthServer != nil {
				healthServer.SetComponentStatus(
					"baseline-saver", "running", "", true,
				)
			}
		}, func(ctx context.Context, progress func()) error {
			return startBaselineSaverWithStatus(
				ctx, manager, baselineCh, 0,
				persistenceStatus(healthServer, "baseline-saver", true, readiness),
				canWrite, progress,
			)
		},
		readiness,
	)
	startRequiredSaver(
		ctx, supervisor, "incident-saver", incidentDone,
		now, func() {
			if healthServer != nil {
				healthServer.SetComponentStatus(
					"incident-saver", "running", "", true,
				)
			}
		}, func(ctx context.Context, progress func()) error {
			return startIncidentSaver(
				ctx, manager, incidentCh,
				persistenceStatus(healthServer, "incident-saver", true, readiness),
				canWrite, progress,
			)
		},
		readiness,
	)
	startOptionalSaver(
		ctx, supervisor, "feedback-saver", feedbackDone, now,
		func() {
			if healthServer != nil {
				healthServer.SetComponentStatus(
					"feedback-saver", "running", "", true,
				)
			}
		},
		func(ctx context.Context, progress func()) error {
			startFeedbackSaver(
				ctx, manager, feedbackCh,
				persistenceStatus(healthServer, "feedback-saver", false, readiness),
				canWrite, now, progress,
			)
			return nil
		},
	)
}

func startRequiredSaver(
	ctx context.Context,
	supervisor *componentSupervisor,
	name string,
	done chan struct{},
	now func() time.Time,
	onHealthy func(),
	run func(context.Context, func()) error,
	readiness *readinessCoordinator,
) {
	progress := newComponentProgress(now())
	supervisor.startOwned(ctx, componentSpec{
		name: name, required: true, progress: progress,
		onHealthy: func() {
			if onHealthy != nil {
				onHealthy()
			}
			if readiness != nil {
				readiness.writerStarted(name)
			}
		},
		run: func(ctx context.Context) error {
			defer close(done)
			return run(ctx, func() { progress.Touch(now()) })
		},
	})
}

func startOptionalSaver(
	ctx context.Context,
	supervisor *componentSupervisor,
	name string,
	done chan struct{},
	now func() time.Time,
	onHealthy func(),
	run func(context.Context, func()) error,
) {
	progress := newComponentProgress(now())
	supervisor.startOwned(ctx, componentSpec{
		name: name, progress: progress,
		onHealthy: onHealthy,
		run: func(ctx context.Context) error {
			defer close(done)
			return run(ctx, func() { progress.Touch(now()) })
		},
	})
}

func restoreChangeTracker(
	ctx context.Context,
	persistenceManager persistence.ChangeHistoryStore,
	tracker *kwcontext.ChangeTracker,
) error {
	changes, err := persistenceManager.LoadChangeHistory(ctx)
	if err == nil && tracker != nil {
		tracker.Restore(changes)
	}
	return err
}

func restoreFeedbackStore(
	ctx context.Context,
	persistenceManager persistence.FeedbackStore,
	store *insight.FeedbackStore,
) error {
	records, err := persistenceManager.LoadRCAFeedback(ctx)
	if err == nil && store != nil {
		store.Restore(records)
	}
	return err
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
		Operation:             persistence.OperationRestore,
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
		Operation:             persistence.OperationRestore,
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
		Operation:             persistence.OperationRestore,
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
	progress func(),
) error {
	return startChangeHistorySaverWithInterval(
		ctx, persistenceManager, tracker, report, canWrite, progress,
		time.Minute,
	)
}

func startChangeHistorySaverWithInterval(
	ctx context.Context,
	persistenceManager persistence.ChangeHistoryStore,
	tracker *kwcontext.ChangeTracker,
	report func(error),
	canWrite func() bool,
	progress func(),
	interval time.Duration,
) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var retryTimer *time.Timer
	var retryC <-chan time.Time
	retryDelay := time.Second
	stopHeartbeat := startProgressHeartbeat(ctx, progress)
	defer stopHeartbeat()
	attempt := func() {
		if !writesAllowed(canWrite) {
			return
		}
		writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := persistenceManager.SaveChangeHistory(
			writeCtx, tracker.Snapshot(),
		)
		cancel()
		if err != nil {
			klog.ErrorS(err, "failed to persist recent change history")
			if report != nil {
				report(err)
			}
			retryTimer = resetRetryTimer(retryTimer, retryDelay)
			retryC = retryTimer.C
			if retryDelay < time.Minute {
				retryDelay *= 2
				if retryDelay > time.Minute {
					retryDelay = time.Minute
				}
			}
			return
		}
		if report != nil {
			report(nil)
		}
		retryDelay = time.Second
		stopRetryTimer(retryTimer)
		retryC = nil
	}
	for {
		if !writesAllowed(canWrite) {
			return errComponentCleanStop
		}
		select {
		case <-ctx.Done():
			if writesAllowed(canWrite) {
				fctx, cancel := finalWriteContext(ctx)
				if err := persistenceManager.SaveChangeHistory(
					fctx, tracker.Snapshot(),
				); err != nil {
					klog.ErrorS(err, "failed to persist final change history")
					if report != nil {
						report(err)
					}
				} else if report != nil {
					report(nil)
				}
				cancel()
			}
			return nil
		case <-ticker.C:
			if progress != nil {
				progress()
			}
			if !writesAllowed(canWrite) {
				return errComponentCleanStop
			}
			attempt()
		case <-retryC:
			attempt()
		}
	}
}

func resetRetryTimer(timer *time.Timer, delay time.Duration) *time.Timer {
	if timer == nil {
		return time.NewTimer(delay)
	}
	stopRetryTimer(timer)
	timer.Reset(delay)
	return timer
}

func stopRetryTimer(timer *time.Timer) {
	if timer == nil || timer.Stop() {
		return
	}
	select {
	case <-timer.C:
	default:
	}
}
