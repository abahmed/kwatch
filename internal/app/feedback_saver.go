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
	done chan<- struct{},
	report func(error),
	canWrite func() bool,
	progress func(),
) {
	defer close(done)
	var pending []insight.RCARecord
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	save := func(writeCtx context.Context, timeout time.Duration) {
		if pending == nil {
			return
		}
		if !writesAllowed(canWrite) {
			pending = nil
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
			return
		}
		if report != nil {
			report(nil)
		}
		cancel()
		pending = nil
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
					save(ctx, 5*time.Second)
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
