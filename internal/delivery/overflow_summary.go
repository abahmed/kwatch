package delivery

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"k8s.io/klog/v2"

	deliveryapi "github.com/abahmed/kwatch/internal/delivery/api"
	"github.com/abahmed/kwatch/internal/metrics"
)

// An overflow summary is the one message that stands in for notifications
// a provider could not take one by one: its queue was full, or its hourly
// budget was spent.
//
// The metric kwatch_delivery_digest_skipped_total
// (Registry.DeliveryDigestSkipped) and the log text name it a "digest".
// Those are external names that dashboards and log searches match on, so
// they stay unchanged.

// overflowSummary accumulates what a saturated queue could not accept.
type overflowSummary struct {
	byReason map[string]int
	total    int
	// unnamed counts notifications whose reason did not fit in byReason
	// (see maxOverflowReasonKeys); they are in total but have no name.
	unnamed int
	// since is when the oldest summarized notification was added.
	since time.Time
	// records are the outbox records of the summarized jobs, kept until
	// the summary is sent: a crash before it does sends them on their
	// own after the restart instead of losing them (maxSummaryRecords).
	records []string
}

// maxSummaryRecords bounds the records one summary keeps; beyond it the
// oldest are given up, as before.
const maxSummaryRecords = 1024

// maxOverflowReasonKeys bounds the distinct reasons one summary stores.
// A provider that stays down for days would otherwise keep one entry for
// every distinct incident title. Later reasons are only counted.
const maxOverflowReasonKeys = 256

// count adds n notifications of reason, or counts them unnamed when the
// summary already holds maxOverflowReasonKeys other reasons.
func (s *overflowSummary) count(reason string, n int) {
	if _, known := s.byReason[reason]; !known &&
		len(s.byReason) >= maxOverflowReasonKeys {
		s.unnamed += n
	} else {
		s.byReason[reason] += n
	}
	s.total += n
}

// addToOverflowSummary records a notification that could not be queued.
func (m *Manager) addToOverflowSummary(
	provider string,
	job deliverJob,
) {
	reason := "message"
	if job.kind == jobIncident && job.incident != nil {
		reason = job.incident.Title
	}
	m.pacer.mu.Lock()
	defer m.pacer.mu.Unlock()
	if m.pacer.summaries == nil {
		m.pacer.summaries = make(map[string]*overflowSummary)
	}
	state := m.pacer.summaries[provider]
	if state == nil {
		state = &overflowSummary{
			byReason: make(map[string]int), since: m.nowTime(),
		}
		m.pacer.summaries[provider] = state
	}
	state.count(reason, 1)
	if job.outboxID != "" {
		state.records = append(state.records, job.outboxID)
		if len(state.records) > maxSummaryRecords {
			m.outbox.Load().remove(state.records[0])
			state.records = state.records[1:]
		}
	}
}

// settleSummaryRecords removes the outbox records a summary kept, once
// it was sent or will never be.
func (m *Manager) settleSummaryRecords(state *overflowSummary) {
	if state == nil {
		return
	}
	box := m.outbox.Load()
	for _, id := range state.records {
		box.remove(id)
	}
	state.records = nil
}

// takeOverflowSummary claims and renders the pending overflow summary for
// a provider. The caller must restore the state when delivery does not
// complete.
func (m *Manager) takeOverflowSummary(
	provider string,
) (*overflowSummary, string) {
	m.pacer.mu.Lock()
	state := m.pacer.summaries[provider]
	if state == nil || state.total == 0 {
		m.pacer.mu.Unlock()
		return nil, ""
	}
	delete(m.pacer.summaries, provider)
	m.pacer.mu.Unlock()
	return state, renderOverflowSummary(state)
}

func (m *Manager) restoreOverflowSummary(
	provider string, state *overflowSummary,
) {
	if state == nil || state.total == 0 {
		return
	}
	m.pacer.mu.Lock()
	defer m.pacer.mu.Unlock()
	current := m.pacer.summaries[provider]
	if current == nil {
		current = &overflowSummary{
			byReason: make(map[string]int), since: state.since,
		}
		m.pacer.summaries[provider] = current
	}
	for reason, count := range state.byReason {
		current.count(reason, count)
	}
	current.unnamed += state.unnamed
	current.total += state.unnamed
	current.records = append(current.records, state.records...)
}

// renderOverflowSummary names the most frequent reasons and counts the
// rest, so a storm reads as one line instead of a wall.
func renderOverflowSummary(state *overflowSummary) string {
	reasons := make([]string, 0, len(state.byReason))
	for reason := range state.byReason {
		reasons = append(reasons, reason)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if state.byReason[reasons[i]] != state.byReason[reasons[j]] {
			return state.byReason[reasons[i]] > state.byReason[reasons[j]]
		}
		return reasons[i] < reasons[j]
	})
	parts := make([]string, 0, maxOverflowSummaryReasons+1)
	for i, reason := range reasons {
		if i == maxOverflowSummaryReasons {
			parts = append(
				parts,
				fmt.Sprintf("+%d other kinds", len(reasons)-i),
			)
			break
		}
		parts = append(
			parts,
			fmt.Sprintf("%s ×%d", reason, state.byReason[reason]),
		)
	}
	if state.unnamed > 0 {
		parts = append(parts, fmt.Sprintf("%d more of other kinds",
			state.unnamed))
	}
	return fmt.Sprintf(
		"%s not sent one by one to avoid flooding this channel: %s.",
		countNotifications(state.total), strings.Join(parts, ", "),
	)
}

func countNotifications(n int) string {
	if n == 1 {
		return "1 notification was"
	}
	return fmt.Sprintf("%d notifications were", n)
}

// flushOverflowSummary delivers any pending overflow summary for a
// provider. It uses the plain-message path every provider implements, and
// it consumes a send slot like any other delivery so the summary itself
// cannot cause a burst.
//
// A provider that skips plain messages (paging, issue trackers) would
// accept the summary and show it to nobody; the summary is counted as
// skipped instead, so the loss stays visible in metrics.
func (m *Manager) flushOverflowSummary(
	ctx context.Context, entry *providerEntry,
) {
	defer m.markBusy()()
	name := entry.provider.Name()
	state, text := m.takeOverflowSummary(name)
	if text == "" {
		return
	}
	if skipsPlainMessages(entry.provider) {
		metrics.DefaultRegistry().DeliveryDigestSkipped.Add(1)
		klog.V(2).InfoS("overflow digest skipped",
			"component", "delivery", "operation", "digest",
			"provider", name, "notifications", state.total)
		m.settleSummaryRecords(state)
		return
	}
	if !m.waitForSendSlot(ctx, name) {
		m.restoreOverflowSummary(name, state)
		return
	}
	// A summary that cannot be sent now stays pending for the next flush;
	// it is neither retried here nor dead-lettered. One the provider
	// rejected for good was dead-lettered by deliver; keeping it would
	// fail and count it again at every flush.
	outcome, _ := m.guardedDeliver(ctx, entry,
		deliverJob{kind: jobMessage, msg: text})
	if outcome == outcomeRetryLater || outcome == outcomeInterrupted ||
		outcome == outcomePanicked {
		m.restoreOverflowSummary(name, state)
		return
	}
	m.settleSummaryRecords(state)
}

// maxOverflowSummaryDelay bounds how long a summary waits for its storm
// to end.
const maxOverflowSummaryDelay = time.Minute

// maybeFlushOverflowSummary sends the provider's overflow summary once its
// queue is empty, or once the summary is a minute old during a long storm.
// Flushing after every job would turn one storm into a summary per job.
func (m *Manager) maybeFlushOverflowSummary(
	ctx context.Context, entry *providerEntry,
) {
	m.pacer.mu.Lock()
	var since time.Time
	pending := false
	if state := m.pacer.summaries[entry.provider.Name()]; state != nil {
		since, pending = state.since, state.total > 0
	}
	m.pacer.mu.Unlock()
	if !pending {
		return
	}
	if m.queuedFor(*entry) > 0 &&
		m.nowTime().Sub(since) < maxOverflowSummaryDelay {
		return
	}
	m.flushOverflowSummary(ctx, entry)
}

// hasOverflowSummary reports whether a summary waits for the provider.
func (m *Manager) hasOverflowSummary(entry providerEntry) bool {
	m.pacer.mu.Lock()
	defer m.pacer.mu.Unlock()
	state := m.pacer.summaries[entry.provider.Name()]
	return state != nil && state.total > 0
}

func skipsPlainMessages(p Provider) bool {
	skipper, ok := p.(deliveryapi.PlainMessageSkipper)
	return ok && skipper.SkipsPlainMessages()
}
