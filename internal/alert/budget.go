package alert

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
)

// Notifications used to leave for the provider as fast as the engine produced
// them. Forty incidents opening in one minute became forty requests, which
// earned a rate-limit response from the chat provider and delayed everything
// queued behind them.
//
// Two things fix that together. Pacing spreads deliveries at a rate providers
// tolerate, with the existing queue absorbing the burst. A digest covers the
// remainder: when the queue is full, what cannot be accepted is summarized in
// one message rather than dropped into the dead-letter queue where nobody
// watching the channel would see it.
const (
	// defaultSendInterval is the minimum gap between two deliveries to the
	// same provider — roughly thirty notifications a minute, comfortably
	// under every provider's published limit.
	defaultSendInterval = 2 * time.Second
	// maxDigestReasons bounds how many distinct reasons a digest names.
	maxDigestReasons = 8
)

// sendPacer holds the per-provider send cadence and overflow digests. It is
// keyed by provider name so providerEntry stays copyable.
type sendPacer struct {
	mu       sync.Mutex
	last     map[string]time.Time
	digests  map[string]*digestState
	interval time.Duration
}

// digestState accumulates what a saturated queue could not accept.
type digestState struct {
	byReason map[string]int
	total    int
}

func (p *sendPacer) sendInterval() time.Duration {
	if p.interval > 0 {
		return p.interval
	}
	return defaultSendInterval
}

// reserve returns how long the caller must wait before delivering to provider,
// and records the resulting send time. Called with the pacer lock held.
func (p *sendPacer) reserve(provider string, now time.Time) time.Duration {
	if p.last == nil {
		p.last = make(map[string]time.Time)
	}
	earliest := p.last[provider].Add(p.sendInterval())
	if !earliest.After(now) {
		p.last[provider] = now
		return 0
	}
	p.last[provider] = earliest
	return earliest.Sub(now)
}

// waitForSendSlot paces deliveries to one provider. It returns false when the
// context ended while waiting, in which case the caller must not deliver.
func (a *AlertManager) waitForSendSlot(
	ctx context.Context,
	provider string,
) bool {
	a.pacer.mu.Lock()
	wait := a.pacer.reserve(provider, a.nowTime())
	a.pacer.mu.Unlock()
	if wait <= 0 {
		return true
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// digestAdd records a notification that could not be queued.
func (a *AlertManager) digestAdd(
	provider string,
	inc *model.Incident,
	action model.IncidentAction,
) {
	if inc == nil {
		return
	}
	reason := inc.Reason
	if reason == "" {
		reason = action.String()
	}
	a.pacer.mu.Lock()
	defer a.pacer.mu.Unlock()
	if a.pacer.digests == nil {
		a.pacer.digests = make(map[string]*digestState)
	}
	state := a.pacer.digests[provider]
	if state == nil {
		state = &digestState{byReason: make(map[string]int)}
		a.pacer.digests[provider] = state
	}
	state.byReason[reason]++
	state.total++
}

// takeDigest removes and renders the pending digest for a provider. The empty
// string means there was nothing pending.
func (a *AlertManager) takeDigest(provider string) string {
	a.pacer.mu.Lock()
	state := a.pacer.digests[provider]
	if state == nil || state.total == 0 {
		a.pacer.mu.Unlock()
		return ""
	}
	delete(a.pacer.digests, provider)
	a.pacer.mu.Unlock()
	return renderDigest(state)
}

// renderDigest names the most frequent reasons and counts the rest, so a
// storm reads as one line instead of a wall.
func renderDigest(state *digestState) string {
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
	parts := make([]string, 0, maxDigestReasons+1)
	for i, reason := range reasons {
		if i == maxDigestReasons {
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
	return fmt.Sprintf(
		":warning: %d notification(s) were not delivered individually "+
			"because the delivery queue was saturated: %s. "+
			"See /incidents for the full list.",
		state.total,
		strings.Join(parts, ", "),
	)
}

// flushDigest delivers any pending overflow summary for a provider. It uses
// the plain-message path every provider implements, and it consumes a send
// slot like any other delivery so the digest itself cannot cause a burst.
func (a *AlertManager) flushDigest(ctx context.Context, entry *providerEntry) {
	name := entry.provider.Name()
	text := a.takeDigest(name)
	if text == "" {
		return
	}
	if !a.waitForSendSlot(ctx, name) {
		return
	}
	if err := entry.provider.SendMessage(text); err != nil {
		klog.ErrorS(err, "failed to deliver notification digest",
			"provider", name)
	}
}
