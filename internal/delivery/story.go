package delivery

import (
	"context"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/api"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/notice"
)

// NotifyStory queues one problem message for every provider. It never
// performs provider I/O on the caller's goroutine.
func (a *Manager) NotifyStory(m notice.Message) {
	klog.V(2).InfoS("queue story", "component", "delivery",
		"conversation", m.Key, "revision", m.Revision)
	story := m
	a.enqueue(deliverJob{kind: jobStory, story: &story})
}

// dispatchStory sends a story through the richest path the provider has:
// native rendering, an alert event keyed by the conversation (so paging
// systems update and resolve one alert), or plain text.
func (a *Manager) dispatchStory(
	ctx context.Context,
	entry *providerEntry,
	job deliverJob,
	opts deliverOpts,
) error {
	p := entry.provider
	m := *job.story
	if sp, ok := p.(api.StoryProvider); ok && opts.prefix == "" {
		return sendWithRetry(ctx, func() error {
			return sp.SendStory(ctx, m)
		}, opts.retry, p.Name())
	}
	templates := entry.templates
	if len(templates) == 0 {
		templates = a.globalTemplates()
	}
	text := opts.prefix + storyText(m, templates)
	if entry.maxBytes > 0 {
		text = truncateMsg(text, entry.maxBytes)
	}
	if _, ok := p.(EventDeliveryProvider); ok {
		ev := storyEvent(m, text)
		return sendWithRetry(ctx, func() error {
			return sendEvent(ctx, p, ev)
		}, opts.retry, p.Name())
	}
	return sendWithRetry(ctx, func() error {
		return sendMessage(ctx, p, text)
	}, opts.retry, p.Name())
}

// storyEvent maps a story onto the alert lifecycle of paging providers:
// one alert per conversation key, resolved by the resolve message.
func storyEvent(m notice.Message, text string) *event.Event {
	action := "firing"
	if m.Status == notice.StatusResolved {
		action = "resolved"
	}
	severity := "warning"
	if m.Status == notice.StatusCritical {
		severity = "critical"
	}
	return &event.Event{
		Narrative: text, Reason: m.Title, Action: action,
		DedupKey: m.Key, Severity: model.Severity(severity),
	}
}
