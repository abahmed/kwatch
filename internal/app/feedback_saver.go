package app

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/insight"
)

type feedbackSaver interface {
	SaveRCAFeedback(context.Context, []insight.RCARecord) error
}

func trySendFeedbackSnapshot(
	ch chan []insight.RCARecord,
	snapshot []insight.RCARecord,
) {
	select {
	case ch <- snapshot:
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- snapshot:
		default:
		}
	}
}

// startFeedbackSaver keeps ConfigMap I/O out of the incident lifecycle hook.
// Feedback is advisory state, so the newest coalesced snapshot is sufficient.
func startFeedbackSaver(
	ctx context.Context,
	persistenceManager feedbackSaver,
	ch <-chan []insight.RCARecord,
	report func(error),
	canWrite func() bool,
	now func() time.Time,
	progress func(),
) {
	var pending []insight.RCARecord
	var retryDelay time.Duration
	var nextRetry time.Time
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	save := func(writeCtx context.Context, timeout time.Duration) {
		if pending == nil {
			return
		}
		if !writesAllowed(canWrite) {
			pending = nil
			retryDelay = 0
			nextRetry = time.Time{}
			return
		}
		currentTime := now()
		if !nextRetry.IsZero() && currentTime.Before(nextRetry) {
			return
		}
		fctx, cancel := context.WithTimeout(writeCtx, timeout)
		err := persistenceManager.SaveRCAFeedback(fctx, pending)
		if err != nil {
			klog.ErrorS(err, "failed to persist RCA feedback")
			if report != nil {
				report(err)
			}
			cancel()
			if retryDelay == 0 {
				retryDelay = time.Second
			} else if retryDelay < time.Minute {
				retryDelay *= 2
				if retryDelay > time.Minute {
					retryDelay = time.Minute
				}
			}
			nextRetry = currentTime.Add(retryDelay)
			return
		}
		if report != nil {
			report(nil)
		}
		cancel()
		pending = nil
		retryDelay = 0
		nextRetry = time.Time{}
	}
	for {
		select {
		case snapshot := <-ch:
			if progress != nil {
				progress()
			}
			pending = snapshot
		case <-ticker.C:
			if progress != nil {
				progress()
			}
			save(ctx, 5*time.Second)
		case <-ctx.Done():
			for {
				select {
				case snapshot := <-ch:
					pending = snapshot
				default:
					fctx, cancel := finalWriteContext(ctx)
					save(fctx, 5*time.Second)
					cancel()
					return
				}
			}
		}
	}
}

func waitFeedbackSaver(deps *serverDeps) bool {
	if deps.feedbackDone == nil {
		return true
	}
	select {
	case <-deps.feedbackDone:
		return true
	case <-time.After(componentShutdownTimeout):
		recordShutdownTimeout("feedback-saver")
		return false
	}
}
