package app

import (
	"encoding/json"
	"sync/atomic"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/storage"
)

// selfReport is kwatch's view of itself for the dashboard overview:
// its sessions, its last contact with the API server, its state file,
// the messages waiting to be sent, the dashboard links, pod log access
// and, while it starts, how far the first sync is. It reads state kwatch
// already holds and is cheap enough to serve per request. It must be
// called after newRunners, which builds the sources.
func (p *pipelineBuild) selfReport() health.SelfReport {
	self := p.gap.self
	out := health.SelfReport{
		StartedAt:        self.startedAt,
		Restarts:         []health.SelfStop{},
		PodLogsForbidden: p.podLogsForbidden(),
	}
	if self.lastStop != nil {
		stop := *self.lastStop
		out.LastStop = &stop
	}
	now := p.clock.Now()
	for _, r := range self.restarts {
		if now.Sub(r.At) < restartWindow {
			out.Restarts = append(out.Restarts, r)
		}
	}
	if p.deps.healthServer != nil {
		out.LastAPIContact = lastRenewalFrom(p.deps)()
	}
	if p.state != nil {
		out.StateFileBytes, _ = p.state.FileSize()
		out.OutboxPending = outboxPending(p.state)
	}
	if p.links != nil {
		out.DashboardLinks = p.links.Status()
	}
	out.Startup = p.startupProgress()
	return out
}

// outboxPending counts the messages the outbox holds: accepted by
// delivery and not yet sent. A read failure counts none.
func outboxPending(state *storage.Store) int {
	n := 0
	_ = storage.OutboxValues[json.RawMessage](state).Range("",
		func(string, json.RawMessage) error {
			n++
			return nil
		})
	return n
}

// startupProgress is how far the first sync of the watches is; nil
// before the sources exist.
func (p *pipelineBuild) startupProgress() *health.StartupProgress {
	if p.source == nil {
		return nil
	}
	typed := p.source.SyncProgress()
	out := &health.StartupProgress{
		Kinds: typed.Kinds, Synced: typed.Synced, Settled: typed.Settled,
		Required: typed.Required, RequiredSynced: typed.RequiredSynced,
		Objects: typed.Objects, Done: typed.Done,
	}
	if p.dynamic != nil {
		dynamic := p.dynamic.Status()
		for _, n := range dynamic.Watched {
			out.DynamicWatched += n
		}
		out.DynamicSynced = dynamic.Synced
	}
	return out
}

// selfHolder hands the leader session's self report to readers outside
// it (the dashboard). The zero value has none.
type selfHolder struct {
	source atomic.Pointer[func() health.SelfReport]
}

// set publishes the session's report; nil withdraws it.
func (h *selfHolder) set(report func() health.SelfReport) {
	if report == nil {
		h.source.Store(nil)
		return
	}
	h.source.Store(&report)
}

// Report is kwatch's view of itself; ok is false while no leader
// session runs a pipeline. Safe for concurrent use.
func (h *selfHolder) Report() (health.SelfReport, bool) {
	report := h.source.Load()
	if report == nil {
		return health.SelfReport{}, false
	}
	return (*report)(), true
}
