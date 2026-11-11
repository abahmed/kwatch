package delivery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

// A provider that cannot deliver must not fail silently. After a short
// grace, one notice goes to every other provider that works, saying
// what is wrong and how many alerts wait; one more goes when the
// provider works again. Nothing repeats while the state is unchanged.
const (
	// noticeGrace is how long a provider must keep failing before
	// kwatch tells the others, so a blip says nothing.
	noticeGrace = 2 * time.Minute
	// noticeEvery is how often failing providers are looked at.
	noticeEvery = 15 * time.Second
)

// providerNotice is what kwatch has said about one provider.
type providerNotice struct {
	failingSince time.Time
	announced    bool
	// waiting is the number of alerts that waited at the last notice.
	waiting int
}

// PlainReason words a fixed health reason for a person.
func PlainReason(reason string) string {
	switch reason {
	case "":
		return ""
	case reasonRejected:
		return "it refused the message. The webhook, token or address " +
			"may have expired"
	case reasonRateLimited:
		return "it is asking kwatch to slow down"
	default:
		return "it isn't answering"
	}
}

// failureNoticeText is the notice for the working providers.
func failureNoticeText(
	name, reason string, since time.Time, waiting int,
) string {
	text := fmt.Sprintf("kwatch can't send to %s since %s: %s.",
		name, since.UTC().Format("15:04 UTC"), PlainReason(reason))
	if waiting > 0 {
		text += fmt.Sprintf(" %d %s waiting.", waiting,
			plural(waiting, "alert is", "alerts are"))
	}
	// Only a refusal is fixed in kwatch's settings: an outage or a rate
	// limit passes on its own, and the setting is fine.
	if reason != reasonRejected {
		return text + " kwatch keeps trying."
	}
	return text + fmt.Sprintf(" Fix: update %s in the installer, "+
		"under Notifications.", name)
}

// recoveryNoticeText is the notice when the provider works again. The
// count is how many waited at the failure notice: some may have aged out
// since, so it says they are being sent, not that all went out.
func recoveryNoticeText(name string, waiting int) string {
	text := fmt.Sprintf("kwatch is sending to %s again.", name)
	if waiting > 0 {
		text += fmt.Sprintf(" The waiting %s %s being sent.",
			plural(waiting, "alert", "alerts"),
			plural(waiting, "is", "are"))
	}
	return text
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// noteFailing remembers when a provider started failing, or lets a
// recovery that was announced wait for its notice. The caller holds
// healthMu.
func (m *Manager) noteFailingLocked(name, reason string, now time.Time) {
	if m.notices == nil {
		m.notices = make(map[string]*providerNotice)
	}
	n := m.notices[name]
	if n == nil {
		n = &providerNotice{}
		m.notices[name] = n
	}
	switch {
	case reason != "" && n.failingSince.IsZero():
		n.failingSince = now
	case reason == "" && !n.announced:
		n.failingSince = time.Time{}
	}
}

// waitingFor counts the alerts waiting for a provider.
func (m *Manager) waitingFor(name string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	depth := len(m.backlog[name])
	if g := m.currentGenerationLocked(); g != nil {
		if entry, ok := g.entries[name]; ok && entry.ch != nil {
			depth += len(entry.ch)
		}
	}
	return depth
}

// noticeSend queues text for one provider only.
func (m *Manager) noticeSend(target, text string) {
	if m.sendNotice != nil {
		m.sendNotice(target, text)
		return
	}
	m.enqueue(deliverJob{kind: jobMessage, msg: text, target: target})
}

// watchProviders looks at the providers until ctx ends.
func (m *Manager) watchProviders(ctx context.Context) {
	ticker := time.NewTicker(noticeEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.CheckNotices()
		case <-m.noticeEventsChannel():
			m.CheckNotices()
		}
	}
}

// CheckNotices sends the failure and recovery notices that are due. It
// is called by the watch loop and by tests, which move the clock.
func (m *Manager) CheckNotices() {
	now := m.nowTime()
	health := m.ProviderHealth()
	working := make([]string, 0, len(health))
	for _, h := range health {
		if h.Reason == "" && !h.LastSuccess.IsZero() {
			working = append(working, h.Name)
		}
	}
	for _, h := range health {
		m.noticeProvider(h, working, now)
	}
	m.logAllFailing(health, working, now)
}

func (m *Manager) noticeProvider(
	h ProviderHealth, working []string, now time.Time,
) {
	waiting := m.waitingFor(h.Name)
	m.healthMu.Lock()
	n := m.notices[h.Name]
	if n == nil {
		m.healthMu.Unlock()
		return
	}
	var text string
	switch {
	case h.Reason != "" && !n.announced && len(working) > 0 &&
		!n.failingSince.IsZero() && now.Sub(n.failingSince) >= noticeGrace:
		n.announced, n.waiting = true, waiting
		text = failureNoticeText(h.Name, h.Reason, n.failingSince, waiting)
	case h.Reason == "" && n.announced:
		text = recoveryNoticeText(h.Name, n.waiting)
		*n = providerNotice{}
	}
	m.healthMu.Unlock()
	if text == "" {
		return
	}
	klog.InfoS("provider notice", "component", "delivery",
		"provider", h.Name, "notice", text)
	for _, other := range working {
		if other != h.Name || h.Reason == "" {
			m.noticeSend(other, text)
		}
	}
}

// logAllFailing logs, on a change only, that no provider can deliver.
func (m *Manager) logAllFailing(
	health []ProviderHealth, working []string, now time.Time,
) {
	m.healthMu.Lock()
	all := len(health) > 0 && len(working) == 0
	if all {
		all = false
		for _, n := range m.notices {
			if !n.failingSince.IsZero() &&
				now.Sub(n.failingSince) >= noticeGrace {
				all = true
			}
		}
	}
	changed := all != m.allFailing
	m.allFailing = all
	m.healthMu.Unlock()
	switch {
	case !changed:
	case all:
		names := make([]string, 0, len(health))
		for _, h := range health {
			names = append(names, h.Name)
		}
		klog.ErrorS(nil, "kwatch can't send alerts anywhere: every "+
			"provider is failing; alerts wait until one works",
			"component", "delivery", "providers", strings.Join(names, ","))
	default:
		klog.InfoS("kwatch can send alerts again",
			"component", "delivery")
	}
}
