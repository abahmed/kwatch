package app

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/pipeline"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
)

// stateRestarts holds the recent starts that followed a session, with
// how it ended, for kwatch's own overview.
const stateRestarts = "restart.history"

// Restart history bounds: the overview shows the last day, and at most
// maxRestartRecords starts are kept whatever their age.
const (
	maxRestartRecords = 32
	restartWindow     = 24 * time.Hour
)

// restartRecord is one start after an earlier session. Its JSON shape
// is persisted.
type restartRecord struct {
	At        time.Time `json:"at"`
	Reason    string    `json:"reason"`
	StoppedAt time.Time `json:"stoppedAt,omitzero"`
}

// restartHistoryStore is the part of persistence that keeps the
// restart history.
type restartHistoryStore interface {
	GetRestarts(context.Context) ([]restartRecord, error)
	SaveRestarts(context.Context, []restartRecord) error
}

// GetRestarts implements restartHistoryStore.
func (d diskState) GetRestarts(context.Context) ([]restartRecord, error) {
	var records []restartRecord
	_, err := d.get(stateRestarts, &records)
	return records, err
}

// SaveRestarts implements restartHistoryStore.
func (d diskState) SaveRestarts(
	_ context.Context, records []restartRecord,
) error {
	return d.put(stateRestarts, records)
}

// stopReason is how the previous session ended, as a restart reason
// code: its own stop record when it wrote one (graceful_shutdown for a
// signal, leader_handoff for a lost Lease, internal_failure), else the
// classified restart reason; unknown when nothing says. "" on a first
// run.
func stopReason(previous runtimeSession, classified string) string {
	switch {
	case previous.SessionID == "":
		return ""
	case previous.EndReason != "":
		return normalizeRestartReason(previous.EndReason)
	case classified != "":
		return classified
	default:
		return "unknown"
	}
}

// recordRestart notes how the previous session ended and adds this
// start to the bounded history. A failure costs only the overview, so
// it is logged.
func (s *startupManager) recordRestart(
	ctx context.Context, previous runtimeSession,
) {
	reason := stopReason(previous, s.restartReason)
	if reason == "" {
		return
	}
	now := s.now()
	s.lastStop = &health.SelfStop{At: now, Reason: reason,
		StoppedAt: previous.EndedAt}
	store, ok := s.persistenceManager.(restartHistoryStore)
	if !ok {
		s.restarts = []health.SelfStop{*s.lastStop}
		return
	}
	records, err := store.GetRestarts(ctx)
	if err != nil {
		klog.V(2).InfoS("restart history unavailable", "error", err)
	}
	records = append(records, restartRecord{At: now, Reason: reason,
		StoppedAt: previous.EndedAt})
	if len(records) > maxRestartRecords {
		records = records[len(records)-maxRestartRecords:]
	}
	if err := store.SaveRestarts(ctx, records); err != nil {
		klog.V(2).InfoS("failed to record restart", "error", err)
	}
	s.restarts = recentRestarts(records, now)
}

// recentRestarts are the records of the last restartWindow, newest
// first.
func recentRestarts(records []restartRecord, now time.Time) []health.SelfStop {
	var out []health.SelfStop
	for i := len(records) - 1; i >= 0; i-- {
		r := records[i]
		if now.Sub(r.At) >= restartWindow {
			continue
		}
		out = append(out, health.SelfStop{At: r.At, Reason: r.Reason,
			StoppedAt: r.StoppedAt})
	}
	return out
}

// startNotice is what the pipeline needs from this start: the gap and
// the startup message that wraps its report, how the start followed
// the previous session, and kwatch's own record for its overview.
type startNotice struct {
	gap     pipeline.Gap
	message func(report string) (string, bool)
	restart announce.Restart
	self    selfSession
}

// selfSession is what this start knows about kwatch's own sessions.
type selfSession struct {
	startedAt time.Time
	lastStop  *health.SelfStop
	restarts  []health.SelfStop
}

// gap is what this start hands the pipeline (startNotice): the
// unmonitored time before it with the sender of the gap message (none
// without a gap), and how it followed the previous session.
func (s *startupManager) gap() startNotice {
	notice := startNotice{self: s.selfSession()}
	if s.lastStop != nil {
		notice.restart = announce.Restart{At: s.lastStop.At,
			Reason: s.lastStop.Reason, Down: s.lastGap}
	}
	if s.downtime <= 0 {
		return notice
	}
	notice.gap = pipeline.Gap{
		Start: s.gapEnd.Add(-s.downtime), End: s.gapEnd,
	}
	notice.message = s.GapMessage
	return notice
}

func (s *startupManager) selfSession() selfSession {
	s.sessionMu.Lock()
	started := s.session.StartedAt
	s.sessionMu.Unlock()
	return selfSession{startedAt: started, lastStop: s.lastStop,
		restarts: append([]health.SelfStop(nil), s.restarts...)}
}
